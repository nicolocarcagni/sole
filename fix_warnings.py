import os
import re

def process_file(path):
    with open(path, 'r') as f:
        content = f.read()

    # core.TxInput{...} -> core.TxInput{Txid: \1, Vout: \2, Signature: \3, PubKey: \4}
    content = re.sub(
        r'core\.TxInput\{\s*([^,{}]+)\s*,\s*([^,{}]+)\s*,\s*([^,{}]+)\s*,\s*([^,{}]+)\s*\}',
        r'core.TxInput{Txid: \1, Vout: \2, Signature: \3, PubKey: \4}',
        content
    )
    # core.TxOutput{...} -> core.TxOutput{Value: \1, PubKeyHash: \2}
    content = re.sub(
        r'core\.TxOutput\{\s*([^,{}]+)\s*,\s*([^,{}]+)\s*\}',
        r'core.TxOutput{Value: \1, PubKeyHash: \2}',
        content
    )
    # core.Transaction{...} -> core.Transaction{ID: \1, Vin: \2, Vout: \3, Timestamp: \4}
    content = re.sub(
        r'core\.Transaction\{\s*([^,{}]+)\s*,\s*([^,{}]+)\s*,\s*([^,{}]+)\s*,\s*([^,{}]+)\s*\}',
        r'core.Transaction{ID: \1, Vin: \2, Vout: \3, Timestamp: \4}',
        content
    )

    with open(path, 'w') as f:
        f.write(content)

process_file('cmd/sole-cli/cli.go')
process_file('pkg/storage/genesis.go')
process_file('pkg/storage/stability_test.go')
process_file('pkg/storage/utxo_set_test.go')
