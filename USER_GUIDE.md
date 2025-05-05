# 📚 BlazeSub User Guide

This guide provides practical information for using BlazeSub in your applications, including configuration options, usage patterns, and best practices.

## 🚀 Getting Started

### Installation

```bash
go get github.com/NSXBet/blazesub
```

### Creating a Bus

BlazeSub provides four methods for creating a bus, giving you flexibility based on your specific needs:

1. **NewBus** - Creates a []byte bus with custom configuration

   ```go
   config := blazesub.Config{
       UseGoroutinePool: true,
       // other configuration options...
   }
   bus, err := blazesub.NewBus(config)
   ```

2. **NewBusWithDefaults** - Creates a []byte bus with default configuration

   ```go
   bus, err := blazesub.NewBusWithDefaults()
   ```

3. **NewBusOf** - Creates a generic bus with custom configuration

   ```go
   type CustomMessage struct {
       Field1 string
       Field2 int
   }

   config := blazesub.Config{
       UseGoroutinePool: false,
       // other configuration options...
   }
   bus, err := blazesub.NewBusOf[CustomMessage](config)
   ```

4. **NewBusWithDefaultsOf** - Creates a generic bus with default configuration

   ```go
   type CustomMessage struct {
       Field1 string
       Field2 int
   }

   bus, err := blazesub.NewBusWithDefaultsOf[CustomMessage]()
   ```

### Basic Usage with []byte Messages

```go
package main

import (
    "fmt"
    "log"

    "github.com/NSXBet/blazesub"
)

func main() {
    // Create a new bus with default configuration, using []byte as the message type
    bus, err := blazesub.NewBusWithDefaults()
    if err != nil {
        log.Fatal(err)
    }
    defer bus.Close()

    // Subscribe to a topic
    subscription, err := bus.Subscribe("sensors/temperature")
    if err != nil {
        log.Fatal(err)
    }

    // Set up message handler
    subscription.OnMessage(blazesub.MessageHandlerFunc[[]byte](func(msg *blazesub.Message[[]byte]) error {
        fmt.Printf("Received message on %s: %s\n", msg.Topic, string(msg.Data))
        return nil
    }))

    // Publish a message
    bus.Publish("sensors/temperature", []byte("25.5"))

    // When done, unsubscribe
    subscription.Unsubscribe()
}
```

### Using Custom Message Types

```go
package main

import (
    "fmt"
    "log"
    "time"

    "github.com/NSXBet/blazesub"
)

// Define your custom message type
type SensorReading struct {
    Value     float64   `json:"value"`
    Unit      string    `json:"unit"`
    DeviceID  string    `json:"device_id"`
    Timestamp time.Time `json:"timestamp"`
}

func main() {
    // Create a bus with your custom type using the generic method
    bus, err := blazesub.NewBusWithDefaultsOf[SensorReading]()
    if err != nil {
        log.Fatal(err)
    }
    defer bus.Close()

    // Subscribe to a topic
    subscription, err := bus.Subscribe("sensors/temperature")
    if err != nil {
        log.Fatal(err)
    }

    // Handle typed messages directly
    subscription.OnMessage(blazesub.MessageHandlerFunc[SensorReading](func(msg *blazesub.Message[SensorReading]) error {
        // Direct access to structured data without deserialization
        reading := msg.Data
        fmt.Printf("Device %s reports %.1f %s at %s\n",
            reading.DeviceID, reading.Value, reading.Unit,
            reading.Timestamp.Format(time.RFC3339))
        return nil
    }))

    // Publish structured data directly
    bus.Publish("sensors/temperature", SensorReading{
        Value:     22.5,
        Unit:      "celsius",
        DeviceID:  "thermostat-living-room",
        Timestamp: time.Now(),
    })

    // When done, unsubscribe
    subscription.Unsubscribe()
}
```

### Using Custom Configuration with Generic Types

```go
package main

import (
    "fmt"
    "log"
    "time"

    "github.com/NSXBet/blazesub"
)

// Define your custom message type
type LogEntry struct {
    Level     string
    Message   string
    Timestamp time.Time
}

func main() {
    // Create custom configuration
    config := blazesub.Config{
        UseGoroutinePool:           false, // Use direct goroutines for better performance
        MaxConcurrentSubscriptions: 50,    // Optimize for your workload
        // Other configuration options...
    }

    // Create a bus with custom type and configuration
    bus, err := blazesub.NewBusOf[LogEntry](config)
    if err != nil {
        log.Fatal(err)
    }
    defer bus.Close()

    // Subscribe and use as normal with your custom type
    subscription, err := bus.Subscribe("logs/system")
    if err != nil {
        log.Fatal(err)
    }

    subscription.OnMessage(blazesub.MessageHandlerFunc[LogEntry](func(msg *blazesub.Message[LogEntry]) error {
        entry := msg.Data
        fmt.Printf("[%s] %s: %s\n",
            entry.Timestamp.Format(time.RFC3339),
            entry.Level,
            entry.Message)
        return nil
    }))

    // Publish a structured message
    bus.Publish("logs/system", LogEntry{
        Level:     "INFO",
        Message:   "System started successfully",
        Timestamp: time.Now(),
    })
}
```

## ⚙️ Configuration Options

BlazeSub can be configured to optimize for different usage patterns:

```go
config := blazesub.Config{
    // Choose between worker pool and direct goroutines for message delivery
    UseGoroutinePool: true,  // true: worker pool, false: direct goroutines

    // Control how many subscribers to process individually vs. in batch
    MaxConcurrentSubscriptions: 50,

    // Size of the worker pool (if using worker pool mode)
    WorkerCount: 10000,

    // Pre-allocate memory for worker pool
    PreAlloc: true,

    // Maximum number of blocking tasks in worker pool queue
    MaxBlockingTasks: 10000,

    // Expiry duration for idle workers (0 means no expiry)
    ExpiryDuration: 0,
}

// Create bus with bytes as the message type
bus, err := blazesub.NewBus(config)

// Or with a custom type
customBus, err := blazesub.NewBusOf[SensorReading](config)
```

### Configuration Parameters Explained

| Parameter                  | Default | Description                                 | Recommendation                                               |
| -------------------------- | ------- | ------------------------------------------- | ------------------------------------------------------------ |
| UseGoroutinePool           | true    | Controls message delivery mode              | Use `false` for max performance, `true` for resource control |
| MaxConcurrentSubscriptions | 10      | Threshold for individual vs. batch delivery | `5-300` for worker pool, `1-750` for direct goroutines       |
| WorkerCount                | 10,000  | Size of worker pool                         | Increase for high concurrency workloads                      |
| PreAlloc                   | true    | Pre-allocate pool memory                    | Keep `true` for best performance                             |
| MaxBlockingTasks           | 10,000  | Queue limit for worker pool                 | Increase for bursty workloads                                |
| ExpiryDuration             | 0       | Worker idle timeout                         | Set to 0 for steady workloads                                |

## 🔖 Topic Format and Wildcards

BlazeSub supports MQTT-compatible topic patterns:

- **Normal topics**: `sensors/temperature`
- **Single-level wildcards** (`+`): `sensors/+/temperature` matches `sensors/living-room/temperature` and `sensors/kitchen/temperature`
- **Multi-level wildcards** (`#`): `sensors/#` matches all topics starting with `sensors/`

### Topic Rules

- Topics are case-sensitive
- Topics can have multiple levels separated by `/`
- `+` matches exactly one level
- `#` must be the last character in the topic and matches all remaining levels

## 🏎️ Performance Optimization

### Choosing the Right Delivery Mode

1. **Direct Goroutines Mode** - Best for:

   - Maximum throughput (up to 84.7 million msgs/sec to 1000 subscribers)
   - Fast message handlers
   - Systems with ample CPU resources

2. **Worker Pool Mode** - Best for:
   - Resource-constrained environments
   - Long-running message handlers
   - Protection against goroutine explosion

### Optimizing MaxConcurrentSubscriptions

This parameter has a significant impact on performance, controlling when BlazeSub switches from individual goroutines to batched processing:

- **Never set it equal to or higher than your max subscriber count** (creates a severe performance cliff)
- For worker pool mode: optimal values are between 5-300
- For direct goroutines mode: optimal values are between 1-750
- Default value (10) works well for most cases

## 📊 Memory Management

BlazeSub is designed for minimal memory usage:

- Core operations use zero allocations
- Message publishing uses only 2 allocations
- Uses 95% less memory than traditional MQTT brokers

### Tips for Memory-Sensitive Applications

1. Use worker pool mode to control goroutine creation
2. Keep MaxConcurrentSubscriptions low (5-50 range)
3. Unsubscribe when no longer needed to free resources

## 🛠️ Common Patterns and Examples

### Wildcard Subscriptions

```go
// Create a bus with byte slice messages
bus, err := blazesub.NewBusWithDefaults()
if err != nil {
    log.Fatal(err)
}

// Subscribe to all temperature sensors
wildcard, err := bus.Subscribe("sensors/+/temperature")
if err != nil {
    log.Fatal(err)
}

wildcard.OnMessage(blazesub.MessageHandlerFunc[[]byte](func(msg *blazesub.Message[[]byte]) error {
    // This will be called for any message matching the pattern
    // e.g., "sensors/living-room/temperature", "sensors/kitchen/temperature", etc.
    return nil
}))
```

### Multiple Subscribers per Topic

```go
// Create a bus with byte slice messages
bus, err := blazesub.NewBusWithDefaults()
if err != nil {
    log.Fatal(err)
}

// Create multiple subscribers to the same topic
for i := 0; i < 5; i++ {
    subscription, err := bus.Subscribe("alerts")
    if err != nil {
        log.Fatal(err)
    }

    // Each subscription gets its own handler
    id := i
    subscription.OnMessage(blazesub.MessageHandlerFunc[[]byte](func(msg *blazesub.Message[[]byte]) error {
        fmt.Printf("Handler %d received: %s\n", id, string(msg.Data))
        return nil
    }))
}

// Publish - all 5 handlers will receive this message
bus.Publish("alerts", []byte("System alert!"))
```

### Error Handling in Message Handlers

```go
bus, err := blazesub.NewBusWithDefaults()
if err != nil {
    log.Fatal(err)
}

subscription, err := bus.Subscribe("topic")
if err != nil {
    log.Fatal(err)
}

subscription.OnMessage(blazesub.MessageHandlerFunc[[]byte](func(msg *blazesub.Message[[]byte]) error {
    // Do some processing
    if err := processMessage(msg.Data); err != nil {
        // Return the error - this doesn't stop message delivery
        // to other subscribers, but can be useful for logging
        return fmt.Errorf("failed to process message: %w", err)
    }
    return nil
}))
```

## 🔍 Troubleshooting

### Common Issues

1. **High Memory Usage**

   - Use worker pool mode instead of direct goroutines
   - Ensure MaxConcurrentSubscriptions is set appropriately
   - Check for subscription leaks (forgotten Unsubscribe calls)

2. **Slow Message Delivery**

   - Use direct goroutines mode for maximum performance
   - Optimize message handlers for quick execution
   - Tune MaxConcurrentSubscriptions parameter

3. **Missing Message Deliveries**
   - Verify topic patterns match exactly (case-sensitive)
   - Ensure wildcard patterns follow MQTT conventions
   - Check for race conditions in message handler setup

## 📈 Performance Monitoring

Monitor these key metrics for optimal performance:

1. **Message throughput**: How many messages per second your system processes
2. **Delivery latency**: Time from publish to handler execution
3. **Memory usage**: Watch for unexpected growth over time
4. **Goroutine count**: Particularly important when using direct goroutines mode

## 🔮 Advanced Usage

### Custom Message Handling

You can implement the `MessageHandler` interface for more complex handling:

```go
// Implementation with a custom type
type CustomHandler[T any] struct {
    // Your fields here
}

var _ blazesub.MessageHandler[[]byte] = (*CustomHandler[[]byte])(nil)

func (h *CustomHandler[T]) OnMessage(msg *blazesub.Message[T]) error {
    // Your custom message handling logic
    return nil
}

// Use it with a byte slice subscription
bus, _ := blazesub.NewBusWithDefaults()
subscription, _ := bus.Subscribe("updates")
subscription.OnMessage(&CustomHandler[[]byte]{})

// Or with a custom type
typedBus, _ := blazesub.NewBusWithDefaultsOf[SensorReading]()
typedSubscription, _ := typedBus.Subscribe("readings")
typedSubscription.OnMessage(&CustomHandler[SensorReading]{})
```

### Combining with Context for Cancellation

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// Create a bus with byte slice messages
bus, err := blazesub.NewBusWithDefaults()
if err != nil {
    log.Fatal(err)
}

// Use context to manage subscription lifetime
go func() {
    subscription, err := bus.Subscribe("updates")
    if err != nil {
        log.Printf("Subscribe error: %v", err)
        return
    }

    subscription.OnMessage(blazesub.MessageHandlerFunc[[]byte](func(msg *blazesub.Message[[]byte]) error {
        // Process message
        return nil
    }))

    // Automatically unsubscribe when context is cancelled
    <-ctx.Done()
    subscription.Unsubscribe()
}()
```

## 📝 Best Practices

1. **Use direct goroutines mode for maximum performance**, unless you need the resource control of worker pools

2. **Optimize message handlers for quick execution** - long-running operations should be offloaded to separate goroutines

3. **Keep MaxConcurrentSubscriptions below your expected subscriber count** to avoid the performance cliff

4. **Always call Unsubscribe when done** to prevent memory leaks

5. **Use wildcard subscriptions judiciously** - too many can impact performance

6. **Use topic hierarchies effectively** to organize your message space (e.g., `app/service/entity/action`)

7. **For complex data structures, use custom types** rather than serializing/deserializing []byte messages

8. **Consider type safety when designing your system** - using generic types can catch errors at compile time
