import re
with open('pkg/p2p/network.go', 'r') as f:
    content = f.read()

content = re.sub(r'type Server struct \{', 'type Server struct {\n\tOnNewTx func(*core.Transaction)\n\tOnNewBlock func(*core.Block)', content)
content = re.sub(r'"github.com/nicolocarcagni/sole/pkg/api"', '', content)

with open('pkg/p2p/network.go', 'w') as f:
    f.write(content)
