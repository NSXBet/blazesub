package blazesub_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/NSXBet/blazesub"
	"github.com/stretchr/testify/require"
)

type PubsubTestPair struct {
	topic   string
	payload []byte
}

func ptp(t *testing.T, values ...any) []PubsubTestPair {
	if len(values)%2 != 0 {
		t.Fatalf("values must be a list of topic and payload pairs")
	}

	ptp := []PubsubTestPair{}
	for i := 0; i < len(values); i += 2 {
		ptp = append(ptp, PubsubTestPair{
			topic:   values[i].(string),
			payload: values[i+1].([]byte),
		})
	}

	return ptp
}

func sub(t *testing.T, topics ...string) []string {
	t.Helper()

	if len(topics) == 0 {
		t.Fatalf("topics must be provided")
	}

	return topics
}

type want map[string][][]byte

type PubsubTestCase struct {
	name      string
	publish   []PubsubTestPair
	subscribe []string
	want      want
}

func TestCanPublishAndSubscribe(t *testing.T) {
	cases := []PubsubTestCase{
		{
			name:      "can publish and subscribe to a single topic",
			publish:   ptp(t, "test", []byte("test-data")),
			subscribe: sub(t, "test"),
			want:      want{"test": {[]byte("test-data")}},
		},
		{
			name: "can publish and subscribe to multiple topics",
			publish: ptp(
				t,
				"test", []byte("test-data"),
				"test2", []byte("test-data2"),
			),
			subscribe: sub(t, "test", "test2"),
			want: want{
				"test":  {[]byte("test-data")},
				"test2": {[]byte("test-data2")},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bus, err := blazesub.NewBusWithDefaults()
			require.NoError(t, err)

			messageHandler := SpyMessageHandler(t)
			for _, topic := range c.subscribe {
				subscription, err := bus.Subscribe(topic)
				require.NoError(t, err)
				require.NotNil(t, subscription)

				subscription.OnMessage(messageHandler)
			}

			for _, pair := range c.publish {
				bus.Publish(pair.topic, pair.payload)
			}

			require.Eventually(t, func() bool {
				return len(messageHandler.MessagesReceived()) == len(c.want)
			}, time.Second, time.Millisecond*10)

			for _, pair := range c.publish {
				require.Len(
					t,
					messageHandler.MessagesReceived()[pair.topic],
					len(c.want[pair.topic]),
				)
				for i, payload := range c.want[pair.topic] {
					require.Equal(
						t,
						payload,
						messageHandler.MessagesReceived()[pair.topic][i].Data,
					)
				}
			}
		})
	}
}

func TestSlowMessageHandler(t *testing.T) {
	bus, err := blazesub.NewBusWithDefaults()
	require.NoError(t, err)

	slowHandler := SpyMessageHandler(t)
	slowHandler.SetDelay(50 * time.Millisecond)

	subscription, err := bus.Subscribe("test")
	require.NoError(t, err)
	require.NotNil(t, subscription)
	subscription.OnMessage(slowHandler)

	fastHandler := SpyMessageHandler(t)

	subscription2, err := bus.Subscribe("test")
	require.NoError(t, err)
	require.NotNil(t, subscription2)
	subscription2.OnMessage(fastHandler)

	bus.Publish("test", []byte("test-data"))

	require.Eventually(t, func() bool {
		return len(fastHandler.MessagesReceived()["test"]) == 1
	}, time.Millisecond*10, time.Millisecond*1)

	require.Equal(t, []byte("test-data"), fastHandler.MessagesReceived()["test"][0].Data)

	// Ensure that closing waits for all messages to be processed.
	bus.Close()

	require.Eventually(t, func() bool {
		return len(slowHandler.MessagesReceived()["test"]) == 1
	}, time.Second, time.Millisecond*10)

	require.Equal(t, 1, len(slowHandler.MessagesReceived()["test"]))
	require.Equal(t, []byte("test-data"), slowHandler.MessagesReceived()["test"][0].Data)
}

type NoOpHandler struct{}

var _ blazesub.MessageHandler = &NoOpHandler{}

func (h *NoOpHandler) OnMessage(message *blazesub.Message) error {
	return nil
}

func BenchmarkPublishAndSubscribe(b *testing.B) {
	bus, err := blazesub.NewBusWithDefaults()
	require.NoError(b, err)

	payload := []byte("test-data")

	messageHandler := &NoOpHandler{}

	b.ResetTimer()

	for i := range b.N {
		subs, err := bus.Subscribe("test-" + strconv.Itoa(i))
		require.NoError(b, err)

		subs.OnMessage(messageHandler)
	}

	for i := range b.N {
		bus.Publish("test-"+strconv.Itoa(i), payload)
	}
}
