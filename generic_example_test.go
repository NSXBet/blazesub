//nolint:sloglint // reason: it's an example file
package blazesub_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/NSXBet/blazesub"
)

// This example demonstrates using BlazeSub with a custom message type.
func ExampleBus_customType() {
	// Define a custom message type
	type SensorReading struct {
		Value     float64   `json:"value"`
		Unit      string    `json:"unit"`
		DeviceID  string    `json:"device_id"`
		Timestamp time.Time `json:"timestamp"`
	}

	// Create a new bus with the custom type
	bus, err := blazesub.NewBusWithDefaultsOf[SensorReading]()
	if err != nil {
		slog.Error("failed to create bus", "error", err)

		return
	}
	defer bus.Close()

	// Create a WaitGroup to wait for messages
	var wg sync.WaitGroup

	wg.Add(1)

	// Subscribe to a topic
	subscription, err := bus.Subscribe("sensors/temperature")
	if err != nil {
		slog.Error("failed to subscribe to topic", "error", err)

		return
	}

	// Set up message handler
	subscription.OnMessage(
		blazesub.MessageHandlerFunc[SensorReading](func(message *blazesub.Message[SensorReading]) error {
			// Access the structured data directly
			reading := message.Data
			fmt.Printf("Received reading: %.1f %s from device %s\n",
				reading.Value, reading.Unit, reading.DeviceID)
			fmt.Printf("Timestamp: %s\n", reading.Timestamp.Format(time.RFC3339))
			wg.Done()

			return nil
		}),
	)

	// Publish a structured message
	bus.Publish("sensors/temperature", SensorReading{
		Value:     22.5,
		Unit:      "°C",
		DeviceID:  "thermostat-123",
		Timestamp: time.Date(2023, 5, 15, 10, 30, 0, 0, time.UTC),
	})

	// Wait for the message to be processed
	wg.Wait()

	// Output:
	// Received reading: 22.5 °C from device thermostat-123
	// Timestamp: 2023-05-15T10:30:00Z
}

// This example demonstrates using BlazeSub with JSON data.
func ExampleBus_jsonData() {
	// Create a new bus with JSON strings as the message type
	bus, err := blazesub.NewBusWithDefaultsOf[string]()
	if err != nil {
		slog.Error("failed to create bus", "error", err)

		return
	}
	defer bus.Close()

	// Create a channel to coordinate the example
	done := make(chan struct{})

	// Subscribe to a topic
	subscription, err := bus.Subscribe("data/json")
	if err != nil {
		slog.Error("failed to subscribe to topic", "error", err)

		return
	}

	// Set up message handler that parses JSON
	subscription.OnMessage(blazesub.MessageHandlerFunc[string](func(message *blazesub.Message[string]) error {
		fmt.Printf("Received JSON data on topic: %s\n", message.Topic)

		// Parse the JSON data
		var data map[string]interface{}
		if uerr := json.Unmarshal([]byte(message.Data), &data); uerr != nil {
			fmt.Printf("Error parsing JSON: %v\n", uerr)

			return uerr
		}

		// Access the parsed data
		fmt.Printf("Name: %s\n", data["name"])
		fmt.Printf("Age: %.0f\n", data["age"])

		if addresses, ok := data["addresses"].([]interface{}); ok {
			fmt.Printf("Number of addresses: %d\n", len(addresses))

			if len(addresses) > 0 {
				address := addresses[0].(map[string]interface{})
				fmt.Printf("First address: %s, %s\n", address["city"], address["country"])
			}
		}

		close(done)

		return nil
	}))

	// Create a JSON message
	jsonData := `{
		"name": "John Doe",
		"age": 30,
		"addresses": [
			{
				"street": "123 Main St",
				"city": "New York",
				"country": "USA"
			},
			{
				"street": "456 Park Ave",
				"city": "Boston",
				"country": "USA"
			}
		]
	}`

	// Publish the JSON data
	bus.Publish("data/json", jsonData)

	// Wait for the message to be processed
	<-done

	// Output:
	// Received JSON data on topic: data/json
	// Name: John Doe
	// Age: 30
	// Number of addresses: 2
	// First address: New York, USA
}

// This example demonstrates using BlazeSub with byte slices (the traditional approach).
func ExampleBus_byteSlices() {
	// Create a new bus with []byte as the message type
	bus, err := blazesub.NewBusWithDefaultsOf[[]byte]()
	if err != nil {
		slog.Error("failed to create bus", "error", err)

		return
	}
	defer bus.Close()

	// Create a channel to coordinate the example
	done := make(chan struct{})

	// Subscribe to a topic
	subscription, err := bus.Subscribe("data/bytes")
	if err != nil {
		slog.Error("failed to subscribe to topic", "error", err)

		return
	}

	// Set up message handler
	subscription.OnMessage(blazesub.MessageHandlerFunc[[]byte](func(message *blazesub.Message[[]byte]) error {
		fmt.Printf("Received data on topic %s: %s\n", message.Topic, string(message.Data))
		close(done)

		return nil
	}))

	// Publish a message
	bus.Publish("data/bytes", []byte("Hello, BlazeSub!"))

	// Wait for the message to be processed
	<-done

	// Output:
	// Received data on topic data/bytes: Hello, BlazeSub!
}
