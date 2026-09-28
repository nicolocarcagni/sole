import re
import sys
with open('/home/nico/.gemini/antigravity/brain/bba0e568-c264-4281-8fa9-e0f079ee84a2/task.md', 'r') as f:
    c = f.read()

c = re.sub(r'\[/\] Resolve circular dependencies.*', r'[x] Resolve circular dependencies', c)
c = re.sub(r'\[ \] Refactor pkg/api.*', r'[x] Refactor pkg/api', c)
c = re.sub(r'\[ \] Refactor pkg/consensus.*', r'[x] Refactor pkg/consensus', c)
c = re.sub(r'\[ \] Refactor pkg/core.*', r'[x] Refactor pkg/core', c)
c = re.sub(r'\[ \] Refactor pkg/p2p.*', r'[x] Refactor pkg/p2p', c)
c = re.sub(r'\[ \] Refactor pkg/storage.*', r'[x] Refactor pkg/storage', c)
c = re.sub(r'\[ \] Refactor pkg/wallet.*', r'[x] Refactor pkg/wallet', c)
c = re.sub(r'\[ \] Refactor cmd/sole-cli.*', r'[x] Refactor cmd/sole-cli', c)
c = re.sub(r'\[ \] Fix broken tests.*', r'[x] Fix broken tests', c)
c = re.sub(r'\[ \] Final compilation and testing.*', r'[x] Final compilation and testing', c)
c = re.sub(r'\[ \] Commit and push changes.*', r'[x] Commit and push changes', c)

with open('/home/nico/.gemini/antigravity/brain/bba0e568-c264-4281-8fa9-e0f079ee84a2/task.md', 'w') as f:
    f.write(c)
