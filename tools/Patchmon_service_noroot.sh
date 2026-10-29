#!/bin/bash
set -e

### =========================================================
### Installation Patchmon sur Laptop
### =========================================================

REPO_DIR="/opt/PatchMon"
WORKDIR="$(pwd)"

### =========================================================
### 1. DEPENDANCES SYSTEME
### =========================================================
echo "[+] Installation des dépendances..."

sudo apt update
sudo apt install -y git golang-go build-essential curl

### =========================================================
### VARIABLES AUTO-ENROLLMENT
### =========================================================

PATCHMON_HOST="patchmon.local"
PATCHMON_TOKEN_KEY="patchmon_xxxxx"
PATCHMON_TOKEN_SECRET="xxxxxxxxx"
PATCHMON_ENROLL_URL="https://${PATCHMON_HOST}/api/v1/auto-enrollment/script?type=direct-host&token_key=${PATCHMON_TOKEN_KEY}&token_secret=${PATCHMON_TOKEN_SECRET}"
echo "[+] Auto-enrollment PatchMon..."
# Auto-enrollment PatchMon
curl -sk "$PATCHMON_ENROLL_URL" | sudo sh

### =========================================================
### 2. UTILISATEUR SYSTEME PATCHMON
### =========================================================
echo "[+] Création utilisateur patchmon..."

if ! id patchmon >/dev/null 2>&1; then
    sudo useradd \
        --system \
        --home-dir /var/lib/patchmon \
        --create-home \
        --shell /usr/sbin/nologin \
        patchmon
fi

### =========================================================
### 2.1 WRAPPER APT -> SUDO APT
### =========================================================
echo "[+] Création wrapper apt -> sudo apt..."

sudo mkdir -p /usr/local/bin

sudo tee /usr/local/bin/apt >/dev/null <<'EOF'
#!/bin/bash
exec sudo /usr/bin/apt "$@"
EOF

sudo tee /usr/local/bin/apt-get >/dev/null <<'EOF'
#!/bin/bash
exec sudo /usr/bin/apt-get "$@"
EOF

sudo chmod +x /usr/local/bin/apt /usr/local/bin/apt-get

sudo mkdir -p /var/lib/patchmon
sudo mkdir -p /var/log/patchmon
sudo mkdir -p /etc/patchmon/logs

sudo chown -R patchmon:patchmon /var/lib/patchmon /var/log/patchmon /etc/patchmon


### =========================================================
### 3. SUDOERS (APT UNIQUEMENT)
### =========================================================
echo "[+] Configuration sudoers patchmon..."

cat <<'EOF' | sudo tee /etc/sudoers.d/patchmon >/dev/null
patchmon ALL=(root) NOPASSWD: /usr/bin/apt, /usr/bin/apt-get, /usr/bin/dpkg, /usr/bin/apt-cache
Defaults:patchmon !requiretty
Defaults:patchmon env_reset
EOF

sudo chmod 440 /etc/sudoers.d/patchmon
sudo visudo -c


### =========================================================
### 4. CLONE / UPDATE
### =========================================================
echo "[+] Récupération du code PatchMon..."

if [ -d "$REPO_DIR" ]; then
    cd "$REPO_DIR"
    git pull
else
    git clone https://github.com/PatchMon/PatchMon.git "$REPO_DIR"
fi


### =========================================================
### 5. DESACTIVE FORCE ROOT
### =========================================================
echo "[+] Patch root check (minimal safe)..."

cd "$REPO_DIR/agent-source-code"

# Patch robuste root check
grep -rl "Geteuid" . | while read f; do
    sed -i 's/os.Geteuid() != 0/false/g' "$f" || true
    sed -i 's/os.Getuid() != 0/false/g' "$f" || true
done

go mod tidy


### =========================================================
### 6. BUILD
### =========================================================
echo "[+] Build PatchMon agent..."

go build -o patchmon-agent ./cmd/patchmon-agent

sudo install -m 0755 patchmon-agent /usr/local/bin/patchmon-agent


### =========================================================
### 7. SYSTEMD
### =========================================================
echo "[+] Installation service systemd..."

cat <<'EOF' | sudo tee /etc/systemd/system/patchmon-agent.service >/dev/null
[Unit]
Description=PatchMon Agent
After=network-online.target

[Service]
Type=simple

User=patchmon
Group=patchmon

WorkingDirectory=/var/lib/patchmon

Environment="PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
Environment=HOME=/var/lib/patchmon
Environment=PATCHMON_ALLOW_NONROOT=1

ExecStart=/usr/local/bin/patchmon-agent serve

Restart=always
RestartSec=5

# IMPORTANT ACCESS
ReadWritePaths=/var/lib/patchmon /var/log/patchmon /etc/patchmon

[Install]
WantedBy=multi-user.target
EOF


### =========================================================
### 8. ACTIVATION
### =========================================================
echo "[+] Activation service..."

sudo systemctl daemon-reload
sudo systemctl enable patchmon-agent
sudo systemctl restart patchmon-agent


### =========================================================
### 9. CLEANUP
### =========================================================
echo "[+] Nettoyage des sources..."

if [ -d "$REPO_DIR" ]; then
    sudo rm -rf "$REPO_DIR"
    echo "[OK] Repo supprimé"
fi

if [ -d "$WORKDIR/PatchMon-main" ]; then
    rm -rf "$WORKDIR/PatchMon-main"
fi

if [ -f "$WORKDIR/PatchMon-main.zip" ]; then
    rm -f "$WORKDIR/PatchMon-main.zip"
fi

echo "[OK] Installation PatchMon terminée avec succès"