package wallet

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/subtle"
	"crypto/x509"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/crypto/argon2"
)

const (
	WalletFile        = "wallet.dat"
	walletFileVersion = byte(1)
	walletSaltSize    = 16
	maxWalletFileSize = 16 << 20
	maxWalletPassword = 1024
)

var (
	walletFileMagic        = []byte{'S', 'O', 'L', 'E', 'W', 'L', 'T', 0}
	ErrLegacyWalletFile    = errors.New("wallet file uses the unencrypted legacy format; run 'sole-cli wallet migrate'")
	ErrWalletDecrypt       = errors.New("unable to decrypt wallet file; check the password or file integrity")
	ErrWalletAlreadySecure = errors.New("wallet file is already encrypted")
)

type Wallets struct {
	Wallets map[string]*Wallet
}

func CreateWallets(password string) (*Wallets, error) {
	wallets := &Wallets{Wallets: make(map[string]*Wallet)}
	if err := wallets.LoadFromFile(password); err != nil {
		return nil, err
	}
	return wallets, nil
}

func (ws *Wallets) AddWallet() (string, string, error) {
	wallet, mnemonic, err := NewWallet()
	if err != nil {
		return "", "", err
	}
	address := wallet.GetAddress()
	ws.Wallets[address] = wallet
	return address, mnemonic, nil
}

func (ws *Wallets) RecoverWallet(mnemonic string) (string, error) {
	wallet, err := MakeWalletFromMnemonic(mnemonic)
	if err != nil {
		return "", err
	}
	address := wallet.GetAddress()
	ws.Wallets[address] = wallet
	return address, nil
}

func (ws *Wallets) ImportWallet(privKeyHex string) (string, error) {
	wallet, err := MakeWalletFromPrivKeyHex(privKeyHex)
	if err != nil {
		return "", err
	}
	address := wallet.GetAddress()
	ws.Wallets[address] = wallet
	return address, nil
}

func (ws *Wallets) RemoveWallet(address string) error {
	if _, ok := ws.Wallets[address]; !ok {
		return errors.New("address not found in wallet file")
	}
	delete(ws.Wallets, address)
	return nil
}

func (ws *Wallets) GetWalletRef(address string) *Wallet {
	return ws.Wallets[address]
}

func (ws *Wallets) GetAddresses() []string {
	addresses := make([]string, 0, len(ws.Wallets))
	for address := range ws.Wallets {
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)
	return addresses
}

func (ws *Wallets) LoadFromFile(password string) error {
	data, err := readWalletFile()
	if err != nil {
		return err
	}
	plaintext, err := decryptWalletFile(data, password)
	if err != nil {
		return err
	}
	wallets, err := decodeWallets(plaintext)
	if err != nil {
		return fmt.Errorf("failed to decode wallet file: %w", err)
	}
	ws.Wallets = wallets
	return nil
}

func (ws *Wallets) SaveToFile(password string) error {
	if err := validateWalletPassword(password); err != nil {
		return err
	}
	var plaintext bytes.Buffer
	gob.Register(elliptic.P256())
	if err := gob.NewEncoder(&plaintext).Encode(ws); err != nil {
		return fmt.Errorf("failed to encode wallets: %w", err)
	}
	if plaintext.Len() > maxWalletFileSize {
		return errors.New("wallet data exceeds maximum file size")
	}
	data, err := encryptWalletFile(plaintext.Bytes(), password)
	if err != nil {
		return err
	}
	return writeWalletFile(data)
}

// MigrateLegacyWalletFile converts the current plaintext gob wallet file to the
// encrypted format. It leaves the original file untouched if validation,
// encryption, or the atomic replacement fails.
func (ws *Wallets) MigrateLegacyWalletFile(password string) error {
	data, err := readWalletFile()
	if err != nil {
		return err
	}
	if len(data) >= len(walletFileMagic) && subtle.ConstantTimeCompare(data[:len(walletFileMagic)], walletFileMagic) == 1 {
		return ErrWalletAlreadySecure
	}
	if err := validateWalletPassword(password); err != nil {
		return err
	}
	wallets, err := decodeWallets(data)
	if err != nil {
		return fmt.Errorf("failed to decode legacy wallet file: %w", err)
	}
	if err := validateWallets(wallets); err != nil {
		return fmt.Errorf("legacy wallet validation failed: %w", err)
	}
	ws.Wallets = wallets
	var plaintext bytes.Buffer
	gob.Register(elliptic.P256())
	if err := gob.NewEncoder(&plaintext).Encode(ws); err != nil {
		return fmt.Errorf("failed to encode wallets: %w", err)
	}
	encrypted, err := encryptWalletFile(plaintext.Bytes(), password)
	if err != nil {
		return err
	}
	return writeWalletFile(encrypted)
}

func decodeWallets(data []byte) (map[string]*Wallet, error) {
	var wallets Wallets
	gob.Register(elliptic.P256())
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&wallets); err != nil {
		return nil, err
	}
	if wallets.Wallets == nil {
		return nil, errors.New("wallet map is missing")
	}
	return wallets.Wallets, nil
}

func readWalletFile() ([]byte, error) {
	file, err := os.Open(WalletFile)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxWalletFileSize+1))
	closeErr := file.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(data) > maxWalletFileSize {
		return nil, errors.New("wallet file exceeds maximum size")
	}
	return data, nil
}

func validateWalletPassword(password string) error {
	if password == "" {
		return errors.New("wallet password must not be empty")
	}
	if len(password) > maxWalletPassword {
		return errors.New("wallet password exceeds maximum length")
	}
	return nil
}

func validateWallets(wallets map[string]*Wallet) error {
	for address, wallet := range wallets {
		if wallet == nil {
			return fmt.Errorf("wallet %q is nil", address)
		}
		privateKey, err := x509.ParseECPrivateKey(wallet.PrivateKey)
		if err != nil {
			return fmt.Errorf("wallet %q has an invalid private key: %w", address, err)
		}
		publicKey := elliptic.Marshal(privateKey.Curve, privateKey.X, privateKey.Y)
		if !bytes.Equal(publicKey, wallet.PublicKey) || wallet.GetAddress() != address {
			return fmt.Errorf("wallet %q has inconsistent key or address data", address)
		}
	}
	return nil
}

func encryptWalletFile(plaintext []byte, password string) ([]byte, error) {
	if err := validateWalletPassword(password); err != nil {
		return nil, err
	}
	salt := make([]byte, walletSaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate wallet salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize wallet encryption: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize wallet authentication: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate wallet nonce: %w", err)
	}
	header := make([]byte, 0, len(walletFileMagic)+1+walletSaltSize)
	header = append(header, walletFileMagic...)
	header = append(header, walletFileVersion)
	header = append(header, salt...)
	ciphertext := aead.Seal(nil, nonce, plaintext, header)
	result := make([]byte, 0, len(header)+len(nonce)+len(ciphertext))
	result = append(result, header...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)
	return result, nil
}

func decryptWalletFile(data []byte, password string) ([]byte, error) {
	if len(data) < len(walletFileMagic) || !bytes.Equal(data[:len(walletFileMagic)], walletFileMagic) {
		return nil, ErrLegacyWalletFile
	}
	minimumSize := len(walletFileMagic) + 1 + walletSaltSize + 12 + 16
	if len(data) < minimumSize {
		return nil, ErrWalletDecrypt
	}
	if data[len(walletFileMagic)] != walletFileVersion {
		return nil, errors.New("unsupported wallet file version")
	}
	if err := validateWalletPassword(password); err != nil {
		return nil, err
	}
	headerSize := len(walletFileMagic) + 1 + walletSaltSize
	salt := data[len(walletFileMagic)+1 : headerSize]
	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrWalletDecrypt
	}
	aead, err := cipher.NewGCM(block)
	if err != nil || len(data) < headerSize+aead.NonceSize()+aead.Overhead() {
		return nil, ErrWalletDecrypt
	}
	nonce := data[headerSize : headerSize+aead.NonceSize()]
	plaintext, err := aead.Open(nil, nonce, data[headerSize+aead.NonceSize():], data[:headerSize])
	if err != nil {
		return nil, ErrWalletDecrypt
	}
	return plaintext, nil
}

func writeWalletFile(data []byte) error {
	dir := filepath.Dir(WalletFile)
	temp, err := os.CreateTemp(dir, ".wallet-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary wallet file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return fmt.Errorf("failed to set wallet file permissions: %w", err)
	}
	written, err := temp.Write(data)
	if err != nil {
		temp.Close()
		return fmt.Errorf("failed to write wallet file: %w", err)
	}
	if written != len(data) {
		temp.Close()
		return io.ErrShortWrite
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("failed to sync wallet file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("failed to close wallet file: %w", err)
	}
	tempInfo, err := os.Stat(tempName)
	if err != nil {
		return fmt.Errorf("failed to verify temporary wallet file: %w", err)
	}
	if tempInfo.Mode().Perm() != 0600 {
		return fmt.Errorf("temporary wallet file permissions are %04o, want 0600", tempInfo.Mode().Perm())
	}
	if err := os.Rename(tempName, WalletFile); err != nil {
		return fmt.Errorf("failed to replace wallet file: %w", err)
	}
	info, err := os.Stat(WalletFile)
	if err != nil {
		return fmt.Errorf("failed to verify wallet file: %w", err)
	}
	if info.Mode().Perm() != 0600 {
		return fmt.Errorf("wallet file permissions are %04o, want 0600", info.Mode().Perm())
	}
	return nil
}
