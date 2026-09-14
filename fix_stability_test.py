import re
with open('pkg/storage/stability_test.go', 'r') as f:
    c = f.read()

c = c.replace('core.NewUTXOTransaction', 'NewUTXOTransaction')
c = c.replace('NewWallet(', 'wallet.NewWallet(')
c = c.replace('walletFile', 'wallet.WalletFile')

with open('pkg/storage/stability_test.go', 'w') as f:
    f.write(c)
