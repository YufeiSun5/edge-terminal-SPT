package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	broker := flag.String("broker", "tcp://127.0.0.1:1883", "MQTT broker URL")
	clientID := flag.String("client-id", "edge-probe", "MQTT client id")
	username := flag.String("username", "", "MQTT username")
	password := flag.String("password", "", "MQTT password")
	localAddr := flag.String("local-addr", "", "local IPv4 address to bind")
	topic := flag.String("topic", "datachange_S_KIO_Project", "MQTT topic to subscribe")
	wait := flag.Duration("wait", 8*time.Second, "wait time after subscribe")
	connectTimeout := flag.Duration("connect-timeout", 2*time.Second, "connect timeout")
	qos := flag.Int("qos", 2, "subscribe qos")
	flag.Parse()

	var messages int64
	opts := MQTT.NewClientOptions()
	opts.AddBroker(*broker)
	opts.SetClientID(fmt.Sprintf("%s_%d", *clientID, time.Now().UnixNano()))
	opts.SetUsername(*username)
	opts.SetPassword(*password)
	opts.SetCleanSession(true)
	opts.SetConnectTimeout(*connectTimeout)
	if strings.TrimSpace(*localAddr) != "" {
		ip := net.ParseIP(*localAddr)
		if ip == nil {
			fmt.Fprintf(os.Stderr, "invalid local addr: %s\n", *localAddr)
			os.Exit(1)
		}
		opts.SetDialer(&net.Dialer{
			Timeout:   *connectTimeout,
			LocalAddr: &net.TCPAddr{IP: ip},
		})
	}
	opts.SetKeepAlive(30 * time.Second)
	opts.SetAutoReconnect(false)
	opts.SetDefaultPublishHandler(func(_ MQTT.Client, msg MQTT.Message) {
		count := atomic.AddInt64(&messages, 1)
		payload := string(msg.Payload())
		if len(payload) > 600 {
			payload = payload[:600] + "...<truncated>"
		}
		log.Printf("message #%d topic=%s payload=%s", count, msg.Topic(), payload)
	})

	client := MQTT.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		fmt.Fprintf(os.Stderr, "connect failed broker=%s username=%s err=%v\n", *broker, displayUser(*username), token.Error())
		os.Exit(2)
	}
	defer client.Disconnect(250)

	if token := client.Subscribe(*topic, byte(*qos), nil); token.Wait() && token.Error() != nil {
		fmt.Fprintf(os.Stderr, "subscribe failed broker=%s topic=%s err=%v\n", *broker, *topic, token.Error())
		os.Exit(3)
	}

	log.Printf("connected broker=%s username=%s topic=%s qos=%d", *broker, displayUser(*username), *topic, *qos)
	time.Sleep(*wait)
	log.Printf("done messages=%d", atomic.LoadInt64(&messages))
}

func displayUser(username string) string {
	if strings.TrimSpace(username) == "" {
		return "<empty>"
	}
	return username
}
