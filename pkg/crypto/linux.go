//go:build linux

package crypto

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"knot/internal/logger"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	ssServiceName   = "org.freedesktop.secrets"
	ssObjectPath    = "/org/freedesktop/secrets"
	ssInterface     = "org.freedesktop.Secret.Service"
	ssCollInterface = "org.freedesktop.Secret.Collection"
	ssPromptIface   = "org.freedesktop.Secret.Prompt"
	ssLoginCollPath = "/org/freedesktop/secrets/collection/login"

	ssInitialTimeout = 3 * time.Second
	ssPromptTimeout  = 2 * time.Minute
)

var ssItemAttributes = map[string]string{
	"service": "knot",
	"account": "knot-master-key",
}

var getSecretServiceKeyFunc = getSecretServiceKey
var getOrCreateSecretServiceKeyFunc = getOrCreateSecretServiceKey

func NewLinuxProvider() (Provider, error) {
	logger.Debug("Initializing Linux crypto provider")

	state, err := LoadState()
	if err != nil {
		return nil, err
	}

	factory, err := newLinuxProviderFactory()
	if err != nil {
		return nil, err
	}

	if state != nil {
		provider, err := factory.providerForState(state.Provider)
		if err != nil {
			return nil, err
		}
		if err := ValidateState(state, provider); err != nil {
			return nil, err
		}
		return provider, nil
	}

	return factory.bootstrapProvider()
}

type linuxProviderFactory struct {
	machineIDKey []byte
}

func newLinuxProviderFactory() (*linuxProviderFactory, error) {
	machineID, err := getMachineID()
	if err != nil {
		return nil, fmt.Errorf("failed to get machine id: %w", err)
	}
	logger.Debug("Machine ID retrieved")

	salt, err := GetSalt()
	if err != nil {
		return nil, fmt.Errorf("failed to get salt: %w", err)
	}

	fallbackKey := DeriveKey(linuxFallbackKeyMaterial(machineID), salt)
	return &linuxProviderFactory{machineIDKey: fallbackKey}, nil
}

func (f *linuxProviderFactory) providerForState(providerID string) (Provider, error) {
	switch providerID {
	case ProviderLinuxSecretService:
		key, err := getSecretServiceKeyFunc(ssInitialTimeout)
		if err != nil {
			return nil, fmt.Errorf("Knot encryption backend is %s, but Secret Service is unavailable or locked. Unlock your keyring and retry, or remove the .crypto-state file to let Knot choose a backend again: %w", ProviderLinuxSecretService, err)
		}
		return newKeyProvider(ProviderLinuxSecretService, key), nil
	case ProviderLinuxMachineID:
		return newKeyProvider(ProviderLinuxMachineID, f.machineIDKey), nil
	default:
		return nil, fmt.Errorf("unknown encryption provider %q", providerID)
	}
}

func (f *linuxProviderFactory) bootstrapProvider() (Provider, error) {
	var candidates []Provider
	var selected Provider
	var reason string
	ssKey, err := getOrCreateSecretServiceKeyFunc(ssInitialTimeout)
	if err != nil {
		logger.Debug("Secret Service access failed, will use Machine ID fallback", "error", err)
		reason = err.Error()
	} else {
		selected = newKeyProvider(ProviderLinuxSecretService, ssKey)
		candidates = append(candidates, selected)
		logger.Debug("Secret Service key retrieved successfully")
	}

	machineProvider := newKeyProvider(ProviderLinuxMachineID, f.machineIDKey)
	if selected == nil {
		selected = machineProvider
	}
	candidates = append(candidates, machineProvider)

	return NewBootstrapProviderWithReason(selected, candidates, NewLinuxProvider, reason), nil
}

type keyProvider struct {
	name string
	key  []byte
}

func newKeyProvider(name string, key []byte) Provider {
	return &keyProvider{name: name, key: key}
}

func (p *keyProvider) Name() string {
	return p.name
}

func (p *keyProvider) Encrypt(plaintext []byte) ([]byte, error) {
	return EncryptWithKey(plaintext, p.key)
}

func (p *keyProvider) Decrypt(ciphertext []byte) ([]byte, error) {
	return DecryptWithKey(ciphertext, p.key)
}

func linuxFallbackKeyMaterial(machineID string) string {
	return machineID + "\x00" + strconv.Itoa(os.Getuid())
}

func getDBusConn() (*dbus.Conn, error) {
	addr := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if addr == "" {
		// Try standard path fallback
		stdPath := fmt.Sprintf("unix:path=/run/user/%d/bus", os.Getuid())
		socketPath := strings.TrimPrefix(stdPath, "unix:path=")
		if _, err := os.Stat(socketPath); err == nil {
			addr = stdPath
			logger.Debug("Using fallback DBUS_SESSION_BUS_ADDRESS", "path", stdPath)
		}
	}

	if addr == "" {
		return nil, fmt.Errorf("no D-Bus session address found (DBUS_SESSION_BUS_ADDRESS is empty)")
	}

	return dbus.Connect(addr)
}

func getSecretServiceKey(timeout time.Duration) ([]byte, error) {
	return secretServiceKey(timeout, false)
}

func getOrCreateSecretServiceKey(timeout time.Duration) ([]byte, error) {
	return secretServiceKey(timeout, true)
}

func secretServiceKey(timeout time.Duration, allowCreate bool) ([]byte, error) {
	conn, err := getDBusConn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	obj := conn.Object(ssServiceName, ssObjectPath)

	// 1. Open Session (Plain)
	var sessionPath dbus.ObjectPath
	var outVariant dbus.Variant
	err = obj.CallWithContext(ctx, ssInterface+".OpenSession", 0, "plain", dbus.MakeVariant("")).Store(&outVariant, &sessionPath)
	if err != nil {
		return nil, fmt.Errorf("OpenSession failed: %w", err)
	}

	// 2. Search Items
	var unlockedPaths []dbus.ObjectPath
	var lockedPaths []dbus.ObjectPath
	err = obj.CallWithContext(ctx, ssInterface+".SearchItems", 0, ssItemAttributes).Store(&unlockedPaths, &lockedPaths)
	if err != nil {
		return nil, fmt.Errorf("SearchItems failed: %w", err)
	}

	itemPath := dbus.ObjectPath("")
	if len(unlockedPaths) > 0 {
		itemPath = unlockedPaths[0]
	} else if len(lockedPaths) > 0 {
		unlocked, err := unlockSecretServiceItems(ctx, conn, obj, lockedPaths)
		if err != nil {
			return nil, fmt.Errorf("Unlock failed: %w", err)
		}
		if len(unlocked) > 0 {
			itemPath = unlocked[0]
		} else {
			return nil, fmt.Errorf("Unlock completed without unlocking matching item")
		}
	}

	if itemPath != "" {
		// 3. Get Secret
		type Secret struct {
			Session     dbus.ObjectPath
			Parameters  []byte
			Value       []byte
			ContentType string
		}
		var secret Secret
		err = conn.Object(ssServiceName, itemPath).CallWithContext(ctx, "org.freedesktop.Secret.Item.GetSecret", 0, sessionPath).Store(&secret)
		if err == nil && len(secret.Value) > 0 {
			return base64.StdEncoding.DecodeString(strings.TrimSpace(string(secret.Value)))
		}
	}

	if !allowCreate {
		return nil, fmt.Errorf("Secret Service item not found")
	}

	// 4. Create new key if not found
	logger.Debug("No key found in Secret Service, creating new one")
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	keyStr := base64.StdEncoding.EncodeToString(key)

	type SecretInput struct {
		Session     dbus.ObjectPath
		Parameters  []byte
		Value       []byte
		ContentType string
	}

	secretInput := SecretInput{
		Session:     sessionPath,
		Parameters:  []byte{},
		Value:       []byte(keyStr),
		ContentType: "text/plain",
	}

	properties := map[string]dbus.Variant{
		"org.freedesktop.Secret.Item.Label":      dbus.MakeVariant("Knot Master Key"),
		"org.freedesktop.Secret.Item.Attributes": dbus.MakeVariant(ssItemAttributes),
	}

	collections, collectionsErr := getSecretServiceCollections(ctx, obj)
	if collectionsErr != nil {
		logger.Debug("Failed to enumerate Secret Service collections", "error", collectionsErr)
	}

	var createErrs []string
	for _, collectionPath := range collections {
		_, err = createSecretServiceItem(ctx, conn, collectionPath, properties, secretInput)
		if err == nil {
			return key, nil
		}
		createErrs = append(createErrs, fmt.Sprintf("%s: %v", collectionPath, err))
	}

	if len(createErrs) > 0 {
		return nil, fmt.Errorf("CreateItem failed: %s", strings.Join(createErrs, "; "))
	}
	if collectionsErr != nil {
		return nil, fmt.Errorf("CreateItem failed: no Secret Service collection found: %w", collectionsErr)
	}
	return nil, fmt.Errorf("CreateItem failed: no Secret Service collection found")
}

func unlockSecretServiceItems(ctx context.Context, conn *dbus.Conn, obj dbus.BusObject, lockedPaths []dbus.ObjectPath) ([]dbus.ObjectPath, error) {
	var unlocked []dbus.ObjectPath
	var prompt dbus.ObjectPath
	if err := obj.CallWithContext(ctx, ssInterface+".Unlock", 0, lockedPaths).Store(&unlocked, &prompt); err != nil {
		return nil, err
	}
	if len(unlocked) > 0 || !validSecretServicePath(prompt) {
		return unlocked, nil
	}

	result, err := completeSecretServicePrompt(ctx, conn, prompt)
	if err != nil {
		return nil, err
	}
	return secretServiceObjectPathsFromVariant(result)
}

func createSecretServiceItem(ctx context.Context, conn *dbus.Conn, collectionPath dbus.ObjectPath, properties map[string]dbus.Variant, secretInput any) (dbus.ObjectPath, error) {
	var newItem dbus.ObjectPath
	var prompt dbus.ObjectPath
	if err := conn.Object(ssServiceName, collectionPath).CallWithContext(ctx, ssCollInterface+".CreateItem", 0, properties, secretInput, true).Store(&newItem, &prompt); err != nil {
		return "", err
	}
	if validSecretServicePath(newItem) {
		return newItem, nil
	}
	if !validSecretServicePath(prompt) {
		return "", fmt.Errorf("CreateItem returned no item and no prompt")
	}

	result, err := completeSecretServicePrompt(ctx, conn, prompt)
	if err != nil {
		return "", err
	}
	itemPath, err := secretServiceObjectPathFromVariant(result)
	if err != nil {
		return "", err
	}
	if !validSecretServicePath(itemPath) {
		return "", fmt.Errorf("CreateItem prompt returned no item")
	}
	return itemPath, nil
}

func completeSecretServicePrompt(ctx context.Context, conn *dbus.Conn, promptPath dbus.ObjectPath) (dbus.Variant, error) {
	sigCh := make(chan *dbus.Signal, 4)
	conn.Signal(sigCh)
	defer conn.RemoveSignal(sigCh)

	matchOptions := []dbus.MatchOption{
		dbus.WithMatchObjectPath(promptPath),
		dbus.WithMatchInterface(ssPromptIface),
		dbus.WithMatchMember("Completed"),
	}
	if err := conn.AddMatchSignalContext(ctx, matchOptions...); err != nil {
		return dbus.Variant{}, err
	}
	defer func() {
		_ = conn.RemoveMatchSignalContext(context.Background(), matchOptions...)
	}()

	if err := conn.Object(ssServiceName, promptPath).CallWithContext(ctx, ssPromptIface+".Prompt", 0, "").Store(); err != nil {
		return dbus.Variant{}, err
	}

	for {
		select {
		case <-ctx.Done():
			return dbus.Variant{}, ctx.Err()
		case sig := <-sigCh:
			if sig == nil || sig.Path != promptPath || sig.Name != ssPromptIface+".Completed" {
				continue
			}
			if len(sig.Body) != 2 {
				return dbus.Variant{}, fmt.Errorf("Prompt.Completed returned %d values, want 2", len(sig.Body))
			}
			dismissed, ok := sig.Body[0].(bool)
			if !ok {
				return dbus.Variant{}, fmt.Errorf("Prompt.Completed dismissed value has unexpected type %T", sig.Body[0])
			}
			if dismissed {
				return dbus.Variant{}, fmt.Errorf("prompt dismissed")
			}
			result, ok := sig.Body[1].(dbus.Variant)
			if !ok {
				return dbus.Variant{}, fmt.Errorf("Prompt.Completed result has unexpected type %T", sig.Body[1])
			}
			return result, nil
		}
	}
}

func secretServiceObjectPathsFromVariant(v dbus.Variant) ([]dbus.ObjectPath, error) {
	paths, ok := v.Value().([]dbus.ObjectPath)
	if !ok {
		return nil, fmt.Errorf("prompt result has unexpected type %T", v.Value())
	}
	result := make([]dbus.ObjectPath, 0, len(paths))
	for _, path := range paths {
		if validSecretServicePath(path) {
			result = append(result, path)
		}
	}
	return result, nil
}

func secretServiceObjectPathFromVariant(v dbus.Variant) (dbus.ObjectPath, error) {
	path, ok := v.Value().(dbus.ObjectPath)
	if !ok {
		return "", fmt.Errorf("prompt result has unexpected type %T", v.Value())
	}
	return path, nil
}

func validSecretServicePath(path dbus.ObjectPath) bool {
	return path != "" && path != "/"
}

func getSecretServiceCollections(ctx context.Context, obj dbus.BusObject) ([]dbus.ObjectPath, error) {
	var defaultPath dbus.ObjectPath
	if err := obj.CallWithContext(ctx, ssInterface+".ReadAlias", 0, "default").Store(&defaultPath); err != nil {
		return nil, fmt.Errorf("ReadAlias(default) failed: %w", err)
	}

	var collectionsVariant dbus.Variant
	if err := obj.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, ssInterface, "Collections").Store(&collectionsVariant); err != nil {
		return nil, fmt.Errorf("Get(Collections) failed: %w", err)
	}

	collections, ok := collectionsVariant.Value().([]dbus.ObjectPath)
	if !ok {
		return nil, fmt.Errorf("Collections property has unexpected type %T", collectionsVariant.Value())
	}

	return secretServiceCollectionCandidates(defaultPath, collections), nil
}

func secretServiceCollectionCandidates(defaultPath dbus.ObjectPath, collections []dbus.ObjectPath) []dbus.ObjectPath {
	seen := make(map[dbus.ObjectPath]bool, len(collections)+2)
	result := make([]dbus.ObjectPath, 0, len(collections)+2)

	add := func(path dbus.ObjectPath) {
		if !validSecretServicePath(path) || seen[path] {
			return
		}
		seen[path] = true
		result = append(result, path)
	}

	add(defaultPath)
	for _, path := range collections {
		add(path)
	}
	add(ssLoginCollPath)

	return result
}

func getMachineID() (string, error) {
	paths := []string{"/etc/machine-id", "/var/lib/dbus/machine-id"}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(data)), nil
		}
	}
	return "", fmt.Errorf("could not find machine-id in any of %v", paths)
}
