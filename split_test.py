import re
with open('pkg/consensus/consensus_test.go', 'r') as f:
    c = f.read()

# We need to extract TestForgeBlock_*, TestAddBlock_*, setupTestDB, initTestChain
# and put them in pkg/storage/blockchain_test.go.

storage_funcs = ['setupTestDB', 'initTestChain', 'TestForgeBlock_HashConsistency', 'TestForgeBlock_ValidatorSet', 'TestForgeBlock_ProofOfWork', 'TestForgeBlock_SignatureValid', 'TestAddBlock_AcceptsValidBlock', 'TestAddBlock_RejectsHashMismatch']

# This is getting a bit tricky with python regex for parsing Go functions.
# I will use a simple awk or just split it manually.
