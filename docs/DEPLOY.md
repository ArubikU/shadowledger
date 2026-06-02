# Running a ShadowLedger node

## Just join mainnet (zero config)

The binary ships with mainnet baked in (genesis + bootstrap seeds), like `bitcoind`. Nothing to
write:

```
go install github.com/ArubikU/shadowledger/cmd/slnode@latest
go install github.com/ArubikU/shadowledger/cmd/slctl@latest

slnode                      # derives the shared genesis, connects to seeds, fast-syncs, serves shards
slctl balance --addr sl…    # slctl talks to your local node, else falls back to the mainnet seed
```

Data lives in `~/.shadowledger`. A fresh node auto-creates its identity key, derives the same
genesis as everyone (deterministic), then syncs from the seed and starts storing its
rendezvous-assigned shards. No `node.yaml` needed.

## Hosting a public/bootstrap node (VPS)

A node is reachable by others only if it has a public IP and the two ports are open. Example on
Oracle Linux 9 (systemd + firewalld), which is how the canonical bootstrap node runs:

```
# copy the binary and (for a validator) its wallet to the box, then:
sudo cp slnode slctl /usr/local/bin/ && sudo restorecon -v /usr/local/bin/slnode  # SELinux label

cat > ~/sl/node.yaml <<YAML
data_dir: /home/opc/sl/data
advertise: "<YOUR_PUBLIC_IP>"     # how peers reach you
node_key: /home/opc/sl/founder.tok   # validator wallet (followers can omit -> auto identity)
YAML
# genesis/validators/seeds are embedded; you only override identity + advertise.

# passphrase for an encrypted .tok node key (avoid the "shadow" filename — SELinux labels it shadow_t):
echo 'SL_WALLET_PASS=…' | sudo tee /etc/sysconfig/sledger >/dev/null
sudo chmod 600 /etc/sysconfig/sledger && sudo restorecon -v /etc/sysconfig/sledger

sudo firewall-cmd --permanent --add-port=4004/tcp --add-port=4005/tcp && sudo firewall-cmd --reload
```

systemd unit `/etc/systemd/system/shadowledger.service`:

```ini
[Unit]
Description=ShadowLedger node
After=network-online.target
Wants=network-online.target
[Service]
User=opc
WorkingDirectory=/home/opc/sl
EnvironmentFile=/etc/sysconfig/sledger
ExecStart=/usr/local/bin/slnode --config /home/opc/sl/node.yaml
Restart=always
RestartSec=3
[Install]
WantedBy=multi-user.target
```

```
sudo systemctl daemon-reload && sudo systemctl enable --now shadowledger
journalctl -u shadowledger -f
```

### Cloud firewall (the part SSH can't do)

OS firewalld is not enough on most clouds — the provider's network layer also filters ingress. On
**Oracle Cloud**: Console → Networking → Virtual Cloud Networks → your VCN → Security Lists →
Default Security List → **Add Ingress Rules**:

- Source CIDR `0.0.0.0/0`, IP Protocol **TCP**, Destination port range **4004,4005** (stateless: no).

(AWS: security-group inbound; GCP: VPC firewall rule. Same idea.) Until this is added, the node runs
and is reachable locally but the world cannot connect.

## Gotchas seen in the wild

- **SELinux (Oracle/RHEL/Fedora):** run binaries from `/usr/local/bin` (label `bin_t`) and
  `restorecon` env files. A binary or EnvironmentFile left under `/home` is `home_t` and systemd
  (init_t) refuses it: *"Failed to load environment files: Permission denied"*.
- **Never name the env file `*shadow*`** — SELinux relabels it `shadow_t`, unreadable by systemd.
- **Encrypted `.tok` node key** requires `SL_WALLET_PASS`; a plaintext `.json` identity key does not
  (fine for non-validator nodes).
