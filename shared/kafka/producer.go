// shared/kafka/producer.go
package kafka

import (
	"encoding/json"
	"log"
	"time"

	"github.com/IBM/sarama"
)

type Producer struct {
	producer sarama.AsyncProducer
}

func NewProducer(brokers string) *Producer {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForLocal       // Balance speed and reliability
	config.Producer.Compression = sarama.CompressionSnappy   // Better throughput
	config.Producer.Flush.Frequency = 500 * time.Millisecond // Batch messages
	config.Producer.Retry.Max = 5

	producer, err := sarama.NewAsyncProducer([]string{brokers}, config)
	if err != nil {
		panic("failed to start Kafka producer: " + err.Error())
	}

	// Handle errors in separate goroutine
	go func() {
		for err := range producer.Errors() {
			log.Printf("Failed to send Kafka message: %v", err)
		}
	}()

	return &Producer{producer: producer}
}

func (p *Producer) Publish(topic string, message map[string]interface{}) {
	bytes, _ := json.Marshal(message)
	p.producer.Input() <- &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(bytes),
	}
}

func (p *Producer) Close() error {
	return p.producer.Close()
}
