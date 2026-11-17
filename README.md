# MicDrop

Links to your WhatsApp account as a companion device (like WhatsApp Web) and answers every voice message you receive in a 1:1 chat, right away, with:

> This recipient cannot receive voice messages.

The reply quotes the voice message. Groups, status updates, channels, your own voice messages and forwarded audio files (no `ptt` flag) are ignored. Voice messages older than 30 minutes (`maxReplyAge`) aren't answered either, so a long outage doesn't reply to a backlog all at once.

Built on [whatsmeow](https://github.com/tulir/whatsmeow), an unofficial WhatsApp Web client. WhatsApp doesn't allow unofficial clients, and could ban the number using them.

## Install

Download the binary for your platform from the [latest release](https://github.com/sfiruch/MicDrop/releases/latest): Linux x64 or arm64 (e.g. Raspberry Pi), Windows x64 or macOS arm64. There is nothing else to install.

```
chmod +x micdrop-linux-arm64        # Linux/macOS only
./micdrop-linux-arm64 pair          # scan the QR code, wait until your phone lists the device, then Ctrl+C
./micdrop-linux-arm64               # keep this running
```

On macOS, clear the quarantine flag first: `xattr -d com.apple.quarantine micdrop-darwin-arm64`. To build from source instead: `go build .`

## Usage

```
micdrop [-db micdrop.db] pair [phone]   # link the account: QR code, or a pairing code when a phone number is given
micdrop [-db micdrop.db] [run]          # answer voice messages until stopped
```

The session lives in the SQLite file given by `-db`. Delete it to unlink.

`pair` keeps running after the scan, because the phone only finishes logging in once the new device has received its initial sync. Stop it with Ctrl+C after your phone lists the device under Linked devices, then start `run` (or your service).

Without a session, or after WhatsApp unlinks the device, MicDrop exits with status 3. Configure your service manager not to restart on that status, since only pairing again fixes it.

## Running as a Linux service

A systemd unit, assuming the binary is at `/usr/local/bin/micdrop` and a `micdrop` system user exists (`useradd --system --shell /usr/sbin/nologin micdrop`):

```ini
[Unit]
Description=MicDrop WhatsApp voice message auto-reply
Wants=network-online.target
After=network-online.target

[Service]
User=micdrop
StateDirectory=micdrop
WorkingDirectory=/var/lib/micdrop
ExecStart=/usr/local/bin/micdrop run
Restart=always
RestartSec=30
RestartPreventExitStatus=3

[Install]
WantedBy=multi-user.target
```

Pair as that user, then enable the service:

```
sudo install -d -o micdrop -g micdrop -m 700 /var/lib/micdrop
sudo -u micdrop sh -c 'cd /var/lib/micdrop && /usr/local/bin/micdrop pair'
sudo systemctl enable --now micdrop
```

## Updating

WhatsApp eventually rejects outdated client versions. Update to the latest release, or rebuild after `go get -u go.mau.fi/whatsmeow && go mod tidy`.
