//go:build windows

package crypto

import (
	"fmt"
	"knot/internal/logger"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type windowsProvider struct {
	fallbackKey []byte
}

func NewWindowsProvider() (Provider, error) {
	logger.Debug("Initializing Windows crypto provider")

	machineID, err := getMachineID()
	if err != nil {
		logger.Debug("Failed to get MachineGuid from registry, using hostname as weak ID fallback", "error", err)
		// Last resort: hostname
		machineID, _ = windows.ComputerName()
	}

	salt, err := GetSalt()
	if err != nil {
		return nil, fmt.Errorf("failed to get salt: %w", err)
	}

	fallbackKey := DeriveKey(machineID, salt)

	return &windowsProvider{
		fallbackKey: fallbackKey,
	}, nil
}

func (p *windowsProvider) Name() string {
	return "Windows DPAPI (with Fallback)"
}

// Encrypt encrypts data using Windows DPAPI, falls back to Machine ID if DPAPI fails.
func (p *windowsProvider) Encrypt(plaintext []byte) ([]byte, error) {
	logger.Debug("Attempting encryption using Windows DPAPI")
	
	// Handle empty input explicitly if necessary, though EncryptWithKey handles it.
	if len(plaintext) == 0 {
		return nil, nil
	}

	var dataIn windows.DataBlob
	dataIn.Size = uint32(len(plaintext))
	dataIn.Data = &plaintext[0]

	var dataOut windows.DataBlob
	// CRYPTPROTECT_UI_FORBIDDEN = 0x1
	err := windows.CryptProtectData(&dataIn, nil, nil, 0, nil, 1, &dataOut)
	if err == nil {
		defer windows.LocalFree(windows.Handle(unsafe.Pointer(dataOut.Data)))
		out := make([]byte, dataOut.Size)
		copy(out, unsafe.Slice(dataOut.Data, dataOut.Size))
		logger.Debug("Data encrypted successfully using DPAPI")
		return out, nil
	}

	logger.Debug("DPAPI encryption failed, falling back to Machine ID", "error", err)
	return EncryptWithKey(plaintext, p.fallbackKey)
}

// Decrypt decrypts data using Windows DPAPI, falls back to Machine ID if DPAPI fails.
func (p *windowsProvider) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, nil
	}

	logger.Debug("Attempting decryption using Windows DPAPI")
	var dataIn windows.DataBlob
	dataIn.Size = uint32(len(ciphertext))
	dataIn.Data = &ciphertext[0]

	var dataOut windows.DataBlob
	err := windows.CryptUnprotectData(&dataIn, nil, nil, 0, nil, 1, &dataOut)
	if err == nil {
		defer windows.LocalFree(windows.Handle(unsafe.Pointer(dataOut.Data)))
		out := make([]byte, dataOut.Size)
		copy(out, unsafe.Slice(dataOut.Data, dataOut.Size))
		logger.Debug("Data decrypted successfully using DPAPI")
		return out, nil
	}

	logger.Debug("DPAPI decryption failed, trying Machine ID fallback", "error", err)
	return DecryptWithKey(ciphertext, p.fallbackKey)
}

func getMachineID() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", err
	}
	defer k.Close()

	s, _, err := k.GetStringValue("MachineGuid")
	if err != nil {
		return "", err
	}
	return s, nil
}
