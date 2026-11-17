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

Exit statuses for service managers:

- **3:** no session, or WhatsApp unlinked the device. Only pairing again fixes it, so don't restart.
- **4:** WhatsApp refused the connection: client outdated, temporary ban, another client took over the session, or an unknown connect failure. An immediate restart would most likely be refused again, so retry rarely (e.g. daily) or after updating.
- **1:** anything else, such as no network at startup. Restart after a short delay.

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
RestartPreventExitStatus=3 4

[Install]
WantedBy=multi-user.target
```

systemd doesn't restart on status 4 then, so retry it from a daily timer, for example one that installs the latest release and runs `systemctl is-failed -q micdrop && systemctl restart micdrop`.

Pair as that user, then enable the service:

```
sudo install -d -o micdrop -g micdrop -m 700 /var/lib/micdrop
sudo -u micdrop sh -c 'cd /var/lib/micdrop && /usr/local/bin/micdrop pair'
sudo systemctl enable --now micdrop
```

## Updating

WhatsApp eventually rejects outdated client versions. A weekly workflow updates whatsmeow and, when anything changed, publishes a new patch release, so installing the latest release regularly (e.g. from a daily timer) keeps MicDrop working. To build an update yourself: `go get -u go.mau.fi/whatsmeow && go mod tidy && go build .`
