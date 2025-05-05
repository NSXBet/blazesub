package blazesub_test

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/NSXBet/blazesub"
)

// This example demonstrates creating a Bus with default configuration
// and performing basic publish/subscribe operations.
func Example_basic() {
	// Create a new bus with default configuration
	bus, err := blazesub.NewBusWithDefaults()
	if err != nil {
		log.Fatal(err)
	}
	defer bus.Close()

	// Create a WaitGroup to wait for messages
	var wg sync.WaitGroup
	wg.Add(1)

	// Subscribe to a topic
	subscription, err := bus.Subscribe("sensors/temperature")
	if err != nil {
		log.Fatal(err)
	}

	// Set up message handler
	subscription.OnMessage(blazesub.MessageHandlerFunc(func(message *blazesub.Message) error {
		fmt.Printf("Received message on topic %s: %s\n", message.Topic, string(message.Data))
		wg.Done()
		return nil
	}))

	// Publish a message to the topic
	bus.Publish("sensors/temperature", []byte("25.5°C"))

	// Wait for the message to be processed
	wg.Wait()

	// Unsubscribe when done
	subscription.Unsubscribe()

	// Output:
	// Received message on topic sensors/temperature: 25.5°C
}

// This example shows how to create a Bus with custom configuration,
// specifically using direct goroutines instead of a worker pool.
func Example_directGoroutines() {
	// Create configuration for direct goroutines mode (no worker pool)
	config := blazesub.Config{
		UseGoroutinePool: false, // Use direct goroutines for maximum performance
	}

	// Create a new bus with the configuration
	bus, err := blazesub.NewBus(config)
	if err != nil {
		log.Fatal(err)
	}
	defer bus.Close()

	// Wait for the bus to be ready (will return immediately for direct goroutines)
	err = bus.WaitReady(time.Second)
	if err != nil {
		log.Fatal(err)
	}

	// Create a WaitGroup to wait for messages
	var wg sync.WaitGroup
	wg.Add(1)

	// Subscribe to a topic
	subscription, err := bus.Subscribe("sensors/humidity")
	if err != nil {
		log.Fatal(err)
	}

	// Set up message handler
	subscription.OnMessage(blazesub.MessageHandlerFunc(func(message *blazesub.Message) error {
		fmt.Printf("Received message on topic %s: %s\n", message.Topic, string(message.Data))
		wg.Done()
		return nil
	}))

	// Publish a message to the topic
	bus.Publish("sensors/humidity", []byte("65%"))

	// Wait for the message to be processed
	wg.Wait()

	// Output:
	// Received message on topic sensors/humidity: 65%
}

// This example demonstrates using a worker pool with custom configuration
// for high throughput scenarios.
func Example_workerPool() {
	// Create configuration for worker pool mode
	config := blazesub.Config{
		UseGoroutinePool:           true,  // Use worker pool
		WorkerCount:                10000, // Set worker count
		MaxConcurrentSubscriptions: 100,   // Configure concurrent subscription processing
		MaxBlockingTasks:           5000,  // Maximum tasks that can be queued
	}

	// Create a new bus with the configuration
	bus, err := blazesub.NewBus(config)
	if err != nil {
		log.Fatal(err)
	}
	defer bus.Close()

	// Wait for the bus to be ready before using it
	err = bus.WaitReady(time.Second)
	if err != nil {
		log.Fatal(err)
	}

	// Create a channel to wait for messages
	received := make(chan struct{})

	// Subscribe to a topic
	subscription, err := bus.Subscribe("system/status")
	if err != nil {
		log.Fatal(err)
	}

	// Set up message handler
	subscription.OnMessage(blazesub.MessageHandlerFunc(func(message *blazesub.Message) error {
		fmt.Printf("Received message on topic %s: %s\n", message.Topic, string(message.Data))
		close(received)
		return nil
	}))

	// Publish a message to the topic
	bus.Publish("system/status", []byte("online"))

	// Wait for the message to be processed
	<-received

	// Output:
	// Received message on topic system/status: online
}

// This example shows how to use single-level wildcards (+) in topic subscriptions.
func Example_singleLevelWildcard() {
	// Create a new bus with default configuration
	bus, err := blazesub.NewBusWithDefaults()
	if err != nil {
		log.Fatal(err)
	}
	defer bus.Close()

	// Create a WaitGroup to wait for messages
	var wg sync.WaitGroup
	wg.Add(2) // Expecting 2 messages

	// Subscribe to a topic with a single-level wildcard
	subscription, err := bus.Subscribe("devices/+/temperature")
	if err != nil {
		log.Fatal(err)
	}

	// Track received messages
	receivedMessages := make([]string, 0, 2)
	var mu sync.Mutex

	// Set up message handler
	subscription.OnMessage(blazesub.MessageHandlerFunc(func(message *blazesub.Message) error {
		mu.Lock()
		receivedMessages = append(receivedMessages, fmt.Sprintf("%s: %s", message.Topic, string(message.Data)))
		mu.Unlock()
		wg.Done()
		return nil
	}))

	// Publish messages to matching topics
	bus.Publish("devices/livingroom/temperature", []byte("22.5°C"))
	bus.Publish("devices/kitchen/temperature", []byte("24.0°C"))

	// This won't match the subscription pattern
	bus.Publish("devices/outdoor/humidity", []byte("45%"))

	// Wait for the messages to be processed
	wg.Wait()

	// Print received messages in a deterministic order
	mu.Lock()
	if len(receivedMessages) > 0 && receivedMessages[0] > receivedMessages[len(receivedMessages)-1] {
		receivedMessages[0], receivedMessages[len(receivedMessages)-1] = receivedMessages[len(receivedMessages)-1], receivedMessages[0]
	}
	for _, msg := range receivedMessages {
		fmt.Println(msg)
	}
	mu.Unlock()

	// Output:
	// devices/kitchen/temperature: 24.0°C
	// devices/livingroom/temperature: 22.5°C
}

// This example demonstrates using multi-level wildcards (#) in topic subscriptions.
func Example_multiLevelWildcard() {
	// Create a new bus with default configuration
	bus, err := blazesub.NewBusWithDefaults()
	if err != nil {
		log.Fatal(err)
	}
	defer bus.Close()

	// Create a WaitGroup to wait for messages
	var wg sync.WaitGroup
	wg.Add(3) // Expecting 3 messages

	// Subscribe to a topic with a multi-level wildcard
	subscription, err := bus.Subscribe("sensors/#")
	if err != nil {
		log.Fatal(err)
	}

	// Track received messages
	receivedMessages := make([]string, 0, 3)
	var mu sync.Mutex

	// Set up message handler
	subscription.OnMessage(blazesub.MessageHandlerFunc(func(message *blazesub.Message) error {
		mu.Lock()
		receivedMessages = append(receivedMessages, fmt.Sprintf("%s: %s", message.Topic, string(message.Data)))
		mu.Unlock()
		wg.Done()
		return nil
	}))

	// Publish messages to matching topics
	bus.Publish("sensors/temperature", []byte("21.5°C"))
	bus.Publish("sensors/humidity/indoor", []byte("40%"))
	bus.Publish("sensors/pressure/outdoor/basement", []byte("1013 hPa"))

	// This won't match the subscription pattern
	bus.Publish("devices/light", []byte("on"))

	// Wait for the messages to be processed
	wg.Wait()

	// Sort and print received messages for deterministic output
	mu.Lock()
	// Simple insertion sort for this small array
	for i := 1; i < len(receivedMessages); i++ {
		j := i
		for j > 0 && receivedMessages[j-1] > receivedMessages[j] {
			receivedMessages[j-1], receivedMessages[j] = receivedMessages[j], receivedMessages[j-1]
			j--
		}
	}
	for _, msg := range receivedMessages {
		fmt.Println(msg)
	}
	mu.Unlock()

	// Output:
	// sensors/humidity/indoor: 40%
	// sensors/pressure/outdoor/basement: 1013 hPa
	// sensors/temperature: 21.5°C
}

// This example demonstrates subscribing to multiple topics and using the unsubscribe functionality.
func Example_multipleSubscriptions() {
	// Create a new bus with default configuration
	bus, err := blazesub.NewBusWithDefaults()
	if err != nil {
		log.Fatal(err)
	}
	defer bus.Close()

	// Create subscriptions for multiple topics
	tempSubscription, err := bus.Subscribe("sensors/temperature")
	if err != nil {
		log.Fatal(err)
	}

	humiditySubscription, err := bus.Subscribe("sensors/humidity")
	if err != nil {
		log.Fatal(err)
	}

	// Create a channel to coordinate the example
	tempReceived := make(chan struct{})
	humidityReceived := make(chan struct{})

	// Handler for temperature messages
	tempSubscription.OnMessage(blazesub.MessageHandlerFunc(func(message *blazesub.Message) error {
		fmt.Printf("Temperature: %s\n", string(message.Data))
		close(tempReceived)
		return nil
	}))

	// Handler for humidity messages
	humiditySubscription.OnMessage(blazesub.MessageHandlerFunc(func(message *blazesub.Message) error {
		fmt.Printf("Humidity: %s\n", string(message.Data))
		close(humidityReceived)
		return nil
	}))

	// Publish messages to both topics
	bus.Publish("sensors/temperature", []byte("23.5°C"))

	// Wait for temperature message
	<-tempReceived

	// Unsubscribe from temperature topic
	err = tempSubscription.Unsubscribe()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Unsubscribed from temperature topic")

	// This message won't be received since we unsubscribed
	bus.Publish("sensors/temperature", []byte("24.0°C"))

	// This message will still be received
	bus.Publish("sensors/humidity", []byte("50%"))

	// Wait for the humidity message to be processed
	<-humidityReceived

	// Output:
	// Temperature: 23.5°C
	// Unsubscribed from temperature topic
	// Humidity: 50%
}
