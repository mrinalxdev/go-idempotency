## Why Idempotency so important

The concept of idempotence is extremely important in distributed systems because it’s hard to get really strong guarantees over how many times a command will be invoked or processed.

Because networks are fundamentally unreliable, most distributed systems cannot guarantee exactly-once message delivery or processing, even when using a message broker like RabbitMQ, Azure Service Bus, or Amazon SQS. Most brokers offer at-least-once delivery, relying on the logic to retry processing as many times as necessary until it acknowledges that message processing is complete.

That means if a message fails to process for any reason, it’s going to be retried. Let’s say we have a message handler like the A/V receiver above. If the message is unambiguous like InputBluRay then it’s fairly easy to write code that will handle it as the user intended, no matter how many times the message is reprocessed. On the other hand, if the message is InputNext, it can be very difficult to write logic to fulfill the user intent under conditions of unknown numbers of retries.

In short, if every single message handler in our system is idempotent, we can retry any message as many times as we want, and it won’t affect the overall correctness of the system.

That sounds great…

So why don’t we make all message handlers idempotent?
Because idempotence is hard. Imagine you need to do a fairly simple operation: create a new user in the database, and then publish a UserCreated event to let other parts of the system know what happened.

Seems simple enough, let’s try some pseudocode:

```go
Handle(CreateUser message)
{
  DB.Store(new User())
  Bus.Publish(new UserCreated())
}
```

This looks good in theory, but what if your message broker doesn’t support any form of transaction? (Spoiler alert: most don’t!) If a failure occurs between these two lines of code, then the database record will be created, but the UserCreated message won’t be published. When the message is retried, a new database record will be written, and then the message will be published.

These extra zombie records are created in the database, most of the time duplicating valid records, without any message ever going out to the rest of the system. It can be difficult to even notice this happening, and even more difficult to clean up the mess later on.

So this should be easy to fix, right? Let’s just flip the order of the statements to fix our zombie record problem:

```go
Handle(CreateUser message)
{
    Bus.Publish(new UserCreated())
    DB.Store(new User())
}
```

Now we’ve got the inverse problem. If something bad happens after the publish but before the database call, we produce a ghost message, an announcement to the rest of our system about an event that never really happened. If anyone tries to look up that User, they won’t find it, because it was never created. But the rest of the downstream processes continue to run on that message, perhaps even billing credit cards but failing to actually ship orders!

If you believe transactions will save you, think again. Wrapping the entire message handler in a database transaction only reorders all of the database operations to the end of the process, where the Commit() occurs. Effectively, a database transaction will turn the first code snippet (with the database first) into the second snippet (with the database last) when it executes.

When designing a reliable system you want to think in terms of what would happen if, on any given line of code, someone pulled out the server’s power cable. There are three operations at play here–receiving the incoming message, the database operation, and sending the outgoing message–and due to the lack of a common transaction it’s very difficult to code this without the possibility for zombie records, ghost messages, and other gotchas.