# 🌐 Guida Operativa: Configurare un Bootnode SOLE

Questa guida ti spiega passo dopo passo come configurare un **Nodo Bootnode** (Seed Node) accessibile pubblicamente, permettendo ad altri nodi di connettersi attraverso Internet.

## 📋 Prerequisiti
*   Un server (VPS) con **IP Pubblico** (es. `84.22.10.5`) OPPURE un PC con porte aperte sul router (Port Forwarding TCP 3000).
*   Go 1.22+ installato.

---

## Passo 1: Compilazione
Assicurati di avere l'ultima versione del software.

```bash
cd sole
go build -o sole-cli .
```

## Passo 2: Inizializzazione
Se è la prima volta che avvii il nodo su questa macchina, inizializza il database e il blocco genesi.

```bash
./sole-cli init
```

---

## Passo 3: Avvio del Bootnode 🚀
Esegui questo comando sul server che agirà da Bootstrap.

*   `--port 3000`: La porta TCP da aprire (assicurati che il Firewall/AWS Security Group la permetta).
*   `--public-ip X.X.X.X`: Il tuo IP Pubblico reale.
*   `--listen 0.0.0.0`: Ascolta su tutte le interfacce locali.

```bash
# Sostituisci 84.22.10.5 con il TUO IP PUBBLICO reale
./sole-cli startnode \
  --port 3000 \
  --listen 0.0.0.0 \
  --public-ip 84.22.10.5
```

### Output Atteso
Il nodo si avvierà e mostrerà qualcosa simile a:
```text
Server listening on /ip4/0.0.0.0/tcp/3000 with peer ID 12D3KooW....
🌍 Announcing Public IP: 84.22.10.5
API Server started on http://0.0.0.0:8080
Waiting for connections...
```

**⚠️ Nota Importante**: Copia il **Peer ID** (es. `12D3KooW...`) che vedi all'avvio. Ti servirà per il Passo 4.
*Attenzione: Attualmente il Peer ID cambia ad ogni riavvio del nodo.*

---

## Passo 4: Connettere un Client (Peer)
Ora, da un altro computer (es. il tuo PC di casa), puoi connetterti al Bootnode.

Costruisci la stringa di connessione (Multiaddr):
`/ip4/<TUO_IP_PUBBLICO_BOOTNODE>/tcp/3000/p2p/<PEER_ID_DEL_BOOTNODE>`

Esempio Completo:
`/ip4/84.22.10.5/tcp/3000/p2p/12D3KooWDjW7...`

Esegui il client:
```bash
./sole-cli startnode \
  --port 3001 \
  --bootnodes "/ip4/84.22.10.5/tcp/3000/p2p/12D3KooWDjW7..."
```

*(Nota: Uso `--port 3001` solo se stai testando sulla stessa macchina per evitare conflitti, altrimenti usa pure 3000).*

---

## ✅ Verifica
Se tutto funziona correntemente:
1.  Il **Client** mostrerà: `✅ Connected to bootnode: 12D3K...`
2.  Il **Bootnode** loggherà la nuova connessione.
3.  I blocchi verranno sincronizzati automaticamente.
4.  Le transazioni inviate su uno si propagheranno all'altro.
