import re
with open('pkg/consensus/consensus_test.go', 'r') as f:
    c = f.read()

c = c.replace('package consensus\n', 'package consensus_test\n')
# Prefix exported functions
for fn in ['AuthorizedValidators', 'IsAuthorizedValidator', 'SignBlock', 'VerifyBlockSignature', 'MineBlock', 'CheckProofOfWork', 'ValidateBlockHeader', 'GetValidatorHex']:
    c = re.sub(r'(?<!\.)\b' + fn + r'\b', f'consensus.{fn}', c)

# Add import
if '"github.com/nicolocarcagni/sole/pkg/consensus"' not in c:
    c = c.replace('import (', 'import (\n\t"github.com/nicolocarcagni/sole/pkg/consensus"\n')

with open('pkg/consensus/consensus_test.go', 'w') as f:
    f.write(c)
