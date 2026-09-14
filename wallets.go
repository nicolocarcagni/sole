package main

import (
	"bytes"
	"crypto/elliptic"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"os"
)

const walletFile = "wallet.dat"

type Wallets struct {
	Wallets map[string]*Wallet
}

func CreateWallets() (*Wallets, error) {
	wallets := Wallets{}
	wallets.Wallets = make(map[string]*Wallet)

	err := wallets.LoadFromFile()

	return &wallets, err
}

func (ws *Wallets) AddWallet() (string, string, error) {
	wallet, mnemonic, err := NewWallet()
	if err != nil {
		return "", "", err
	}
	address := fmt.Sprintf("%s", wallet.GetAddress())

	ws.Wallets[address] = wallet

	return address, mnemonic, nil
}

func (ws *Wallets) RecoverWallet(mnemonic string) (string, error) {
	wallet, err := MakeWalletFromMnemonic(mnemonic)
	if err != nil {
		return "", err
	}

	address := fmt.Sprintf("%s", wallet.GetAddress())
	ws.Wallets[address] = wallet

	return address, nil
}

func (ws *Wallets) ImportWallet(privKeyHex string) (string, error) {
	wallet, err := MakeWalletFromPrivKeyHex(privKeyHex)
	if err != nil {
		return "", err
	}

	address := fmt.Sprintf("%s", wallet.GetAddress())
	ws.Wallets[address] = wallet

	return address, nil
}

func (ws *Wallets) RemoveWallet(address string) error {
	if _, ok := ws.Wallets[address]; !ok {
		return fmt.Errorf("Address not found in wallet file")
	}

	delete(ws.Wallets, address)
	return nil
}

func (ws *Wallets) GetWallet(address string) Wallet {
	return *ws.Wallets[address]
}

func (ws *Wallets) GetWalletRef(address string) *Wallet {
	return ws.Wallets[address]
}

func (ws *Wallets) GetAddresses() []string {
	var addresses []string

	for address := range ws.Wallets {
		addresses = append(addresses, address)
	}

	return addresses
}

func (ws *Wallets) LoadFromFile() error {
	if _, err := os.Stat(walletFile); os.IsNotExist(err) {
		return err
	}

	fileContent, err := ioutil.ReadFile(walletFile)
	if err != nil {
		return fmt.Errorf("failed to read wallet file: %w", err)
	}

	var wallets Wallets
	gob.Register(elliptic.P256())
	decoder := gob.NewDecoder(bytes.NewReader(fileContent))
	err = decoder.Decode(&wallets)
	if err != nil {
		return fmt.Errorf("failed to decode wallet file: %w", err)
	}

	ws.Wallets = wallets.Wallets

	return nil
}

func (ws *Wallets) SaveToFile() error {
	var content bytes.Buffer

	gob.Register(elliptic.P256())
	encoder := gob.NewEncoder(&content)
	err := encoder.Encode(ws)
	if err != nil {
		return fmt.Errorf("failed to encode wallets: %w", err)
	}

	err = ioutil.WriteFile(walletFile, content.Bytes(), 0600)
	if err != nil {
		return fmt.Errorf("failed to write wallet file: %w", err)
	}
	return nil
}
