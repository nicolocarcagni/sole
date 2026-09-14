with open('pkg/p2p/mempool_test.go', 'r') as f:
    lines = f.readlines()
with open('pkg/p2p/mempool_test.go', 'w') as f:
    seen = False
    for line in lines:
        if 'github.com/dgraph-io/badger/v3' in line:
            if seen:
                continue
            seen = True
        f.write(line)
