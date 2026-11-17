// MicDrop links to a WhatsApp account as a companion device and answers every incoming voice message in a
// 1:1 chat with a fixed text reply.
//
//	micdrop pair [phone]   link the account (QR code, or pairing code when a phone number is given), then keep
//	                       answering like run until stopped, so the phone can finish its login sync
//	micdrop [run]          answer voice messages until stopped
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

const (
	replyText  = "This recipient cannot receive voice messages."
	deviceName = "MicDrop"

	// Voice messages older than this are not answered. Messages that arrive while MicDrop is offline are delivered
	// on reconnect; this keeps a long outage (or a fresh pairing) from replying to old voice notes in bulk.
	maxReplyAge = 30 * time.Minute

	pairTimeout = 3 * time.Minute

	// Exit code for a missing or unlinked session. Restarting can't fix it, so service managers shouldn't retry
	// (systemd: RestartPreventExitStatus=3).
	exitNotPaired = 3
)

var errNotPaired = errors.New("not paired; link the account with `micdrop pair`")

func main() {
	dbPath := flag.String("db", "micdrop.db", "SQLite file holding the WhatsApp session")
	logLevel := flag.String("log", "INFO", "log level: DEBUG, INFO, WARN or ERROR")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log := waLog.Stdout("MicDrop", *logLevel, false)
	if err := run(ctx, log, *dbPath, flag.Args()); err != nil {
		log.Errorf("%v", err)
		if errors.Is(err, errNotPaired) {
			os.Exit(exitNotPaired)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, log waLog.Logger, dbPath string, args []string) error {
	store.DeviceProps.Os = proto.String(deviceName)

	container, err := sqlstore.New(ctx, "sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", log.Sub("DB"))
	if err != nil {
		return fmt.Errorf("open session store: %w", err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("load device: %w", err)
	}
	client := whatsmeow.NewClient(device, log.Sub("WA"))

	command := "run"
	if len(args) > 0 {
		command = args[0]
	}
	switch command {
	case "pair":
		if device.ID != nil {
			return fmt.Errorf("already paired as %s; delete %s to pair again", device.ID, dbPath)
		}
		phone := ""
		if len(args) > 1 {
			phone = args[1]
		}
		return serve(ctx, log, client, true, phone)
	case "run":
		if device.ID == nil {
			return errNotPaired
		}
		return serve(ctx, log, client, false, "")
	default:
		return fmt.Errorf("unknown command %q; use `pair [phone]` or `run`", command)
	}
}

func pair(ctx context.Context, client *whatsmeow.Client, phone string) error {
	ctx, cancel := context.WithTimeout(ctx, pairTimeout)
	defer cancel()

	qrChan, err := client.GetQRChannel(ctx)
	if err != nil {
		return err
	}
	if err := client.Connect(); err != nil {
		return err
	}

	codeRequested := false
	for evt := range qrChan {
		switch {
		case evt.Event == whatsmeow.QRChannelEventCode && phone == "":
			fmt.Println("WhatsApp → Settings → Linked devices → Link a device, then scan:")
			qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
		case evt.Event == whatsmeow.QRChannelEventCode && !codeRequested:
			code, err := client.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
			if err != nil {
				return fmt.Errorf("request pairing code: %w", err)
			}
			fmt.Printf("WhatsApp → Settings → Linked devices → Link a device → Link with phone number instead, then enter: %s\n", code)
			codeRequested = true
		case evt.Event == whatsmeow.QRChannelEventCode:
			// the pairing code stays valid while the QR code rotates
		case evt == whatsmeow.QRChannelSuccess:
			// Stay connected: the phone only finishes logging in after this client has taken its initial sync.
			fmt.Printf("Paired as %s. Leave this running until your phone has finished logging in and lists the device under Linked devices, then press Ctrl+C.\n", client.Store.ID)
			return nil
		case evt.Error != nil:
			return fmt.Errorf("pairing failed: %w", evt.Error)
		default:
			return fmt.Errorf("pairing failed: %s", evt.Event)
		}
	}
	return fmt.Errorf("pairing aborted: %w", ctx.Err())
}

func serve(ctx context.Context, log waLog.Logger, client *whatsmeow.Client, pairFirst bool, phone string) error {
	loggedOut := make(chan struct{}, 1)
	client.AddEventHandler(func(rawEvt any) {
		switch evt := rawEvt.(type) {
		case *events.Message:
			if !evt.Message.GetAudioMessage().GetPTT() || evt.Info.IsFromMe || evt.Info.IsGroup ||
				evt.Info.Chat.Server != types.DefaultUserServer && evt.Info.Chat.Server != types.HiddenUserServer {
				return
			}
			if age := time.Since(evt.Info.Timestamp); age > maxReplyAge {
				log.Infof("Ignoring voice message %s from %s, it is %s old", evt.Info.ID, evt.Info.Chat, age.Round(time.Second))
				return
			}
			_, err := client.SendMessage(ctx, evt.Info.Chat, &waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					Text: proto.String(replyText),
					ContextInfo: &waE2E.ContextInfo{
						StanzaID:      proto.String(evt.Info.ID),
						Participant:   proto.String(evt.Info.Sender.ToNonAD().String()),
						QuotedMessage: evt.Message,
					},
				},
			})
			if err != nil {
				log.Errorf("Replying to voice message %s from %s failed: %v", evt.Info.ID, evt.Info.Chat, err)
				return
			}
			log.Infof("Replied to voice message %s from %s", evt.Info.ID, evt.Info.Chat)
		case *events.Connected:
			log.Infof("Connected as %s", client.Store.ID)
		case *events.LoggedOut:
			loggedOut <- struct{}{}
		}
	})

	defer client.Disconnect()
	if pairFirst {
		if err := pair(ctx, client, phone); err != nil {
			return err
		}
	} else if err := client.Connect(); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		log.Infof("Shutting down")
		return nil
	case <-loggedOut:
		return fmt.Errorf("logged out by WhatsApp (device unlinked): %w", errNotPaired)
	}
}
