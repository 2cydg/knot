//go:build windows

package crypto

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsProvider struct{}

func NewWindowsProvider() (Provider, error) {
	return &windowsProvider{}, nil
}

// Encrypt encrypts data using Windows DPAPI.
func (p *windowsProvider) Encrypt(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, nil
	}

	var dataIn windows.DataBlob
	dataIn.Size = uint32(len(plaintext))
	dataIn.Data = &plaintext[0]

	var dataOut windows.DataBlob
	// CRYPTPROTECT_UI_FORBIDDEN (0x1) is often preferred for non-interactive tools
	err := windows.CryptProtectData(&dataIn, nil, nil, 0, nil, 1, &dataOut)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(dataOut.Data)))

	// Copy the data out of the memory managed by DPAPI
	out := make([]byte, dataOut.Size)
	copy(out, unsafe.Slice(dataOut.Data, dataOut.Size))
	return out, nil
}

// Decrypt decrypts data using Windows DPAPI.
func (p *windowsProvider) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, nil
	}

	var dataIn windows.DataBlob
	dataIn.Size = uint32(len(ciphertext))
	dataIn.Data = &ciphertext[0]

	var dataOut windows.DataBlob
	err := windows.CryptUnprotectData(&dataIn, nil, nil, 0, nil, 1, &dataOut)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(dataOut.Data)))

	// Copy the data out
	out := make([]byte, dataOut.Size)
	copy(out, unsafe.Slice(dataOut.Data, dataOut.Size))
	return out, nil
}
