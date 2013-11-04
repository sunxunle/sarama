package sarama

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"
)

func TestSimpleMultiProducer(t *testing.T) {
	responses := make(chan []byte, 1)
	extraResponses := make(chan []byte)
	mockBroker := NewMockBroker(t, responses)
	mockExtra := NewMockBroker(t, extraResponses)
	defer mockBroker.Close()
	defer mockExtra.Close()

	// return the extra mock as another available broker
	response := []byte{
		0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x01,
		0x00, 0x09, 'l', 'o', 'c', 'a', 'l', 'h', 'o', 's', 't',
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x01,
		0x00, 0x00,
		0x00, 0x08, 'm', 'y', '_', 't', 'o', 'p', 'i', 'c',
		0x00, 0x00, 0x00, 0x01,
		0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00}
	binary.BigEndian.PutUint32(response[19:], uint32(mockExtra.Port()))
	responses <- response
	go func() {
		msg := []byte{
			0x00, 0x00, 0x00, 0x01,
			0x00, 0x08, 'm', 'y', '_', 't', 'o', 'p', 'i', 'c',
			0x00, 0x00, 0x00, 0x01,
			0x00, 0x00, 0x00, 0x00,
			0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
		binary.BigEndian.PutUint64(msg[23:], 0)
		extraResponses <- msg
	}()

	client, err := NewClient("client_id", []string{mockBroker.Addr()}, nil)
	if err != nil {
		t.Fatal(err)
	}

	producer, err := NewMultiProducer(client, &MultiProducerConfig{
		RequiredAcks:  WaitForLocal,
		MaxBufferTime: 1000000, // "never"
		// So that we flush once, after the 10th message.
		MaxBufferBytes: uint32((len("ABC THE MESSAGE") * 10) - 1),
	})
	defer producer.Close()

	for i := 0; i < 10; i++ {
		err = producer.SendMessage("my_topic", nil, StringEncoder("ABC THE MESSAGE"))
		if err != nil {
			t.Error(err)
		}
	}

	select {
	case err = <-producer.Errors():
		if err != nil {
			t.Error(err)
		}
	case <-time.After(1 * time.Second):
		t.Error(fmt.Errorf("Message was never received"))
	}

	select {
	case <-producer.Errors():
		t.Error(fmt.Errorf("too many values returned"))
	default:
	}

	// TODO: This doesn't really test that we ONLY flush once.
	// For example, change the MaxBufferBytes to be much lower.
}

func TestMultipleMultiProducer(t *testing.T) {
	responses := make(chan []byte, 1)
	responsesA := make(chan []byte)
	responsesB := make(chan []byte)
	mockBroker := NewMockBroker(t, responses)
	mockBrokerA := NewMockBroker(t, responsesA)
	mockBrokerB := NewMockBroker(t, responsesB)
	defer mockBroker.Close()
	defer mockBrokerA.Close()
	defer mockBrokerB.Close()

	// TODO: remove this.
	time.Sleep(10 * time.Millisecond)

	// We're going to return:
	// topic: topic_a; partition: 0; brokerID: 1
	// topic: topic_b; partition: 0; brokerID: 2

	// Return the extra broker metadata so that the producer will send
	// requests to mockBrokerA and mockBrokerB.
	response := []byte{
		0x00, 0x00, 0x00, 0x02, // 0:3 number of brokers

		0x00, 0x00, 0x00, 0x01, // 4:7 broker ID
		0x00, 0x09, 'l', 'o', 'c', 'a', 'l', 'h', 'o', 's', 't', // 8:18 hostname
		0xFF, 0xFF, 0xFF, 0xFF, // 19:22 port will be written here.

		0x00, 0x00, 0x00, 0x02, // 23:26 broker ID
		0x00, 0x09, 'l', 'o', 'c', 'a', 'l', 'h', 'o', 's', 't', // 27:37 hostname
		0xFF, 0xFF, 0xFF, 0xFF, // 38:41 port will be written here.

		0x00, 0x00, 0x00, 0x02, // number of topic metadata records

		0x00, 0x00, // error: 0 means no error
		0x00, 0x07, 't', 'o', 'p', 'i', 'c', '_', 'a', // topic name
		0x00, 0x00, 0x00, 0x01, // number of partition metadata records for this topic
		0x00, 0x00, // error: 0 means no error
		0x00, 0x00, 0x00, 0x00, // partition ID
		0x00, 0x00, 0x00, 0x01, // broker ID of leader
		0x00, 0x00, 0x00, 0x00, // replica set
		0x00, 0x00, 0x00, 0x00, // ISR set

		0x00, 0x00, // error: 0 means no error
		0x00, 0x07, 't', 'o', 'p', 'i', 'c', '_', 'b', // topic name
		0x00, 0x00, 0x00, 0x01, // number of partition metadata records for this topic
		0x00, 0x00, // error: 0 means no error
		0x00, 0x00, 0x00, 0x00, // partition ID
		0x00, 0x00, 0x00, 0x02, // broker ID of leader
		0x00, 0x00, 0x00, 0x00, // replica set
		0x00, 0x00, 0x00, 0x00, // ISR set
	}
	binary.BigEndian.PutUint32(response[19:], uint32(mockBrokerA.Port()))
	binary.BigEndian.PutUint32(response[38:], uint32(mockBrokerB.Port()))
	responses <- response

	go func() {
		msg := []byte{
			0x00, 0x00, 0x00, 0x01, // 0:3 number of topics
			0x00, 0x07, 't', 'o', 'p', 'i', 'c', '_', 'a', // 4:12 topic name
			0x00, 0x00, 0x00, 0x01, // 13:16 number of blocks for this topic
			0x00, 0x00, 0x00, 0x00, // 17:20 partition id
			0x00, 0x00, // 21:22 error: 0 means no error
			0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, // 23:30 offset
		}
		binary.BigEndian.PutUint64(msg[23:], 0)
		responsesA <- msg
	}()

	go func() {
		msg := []byte{
			0x00, 0x00, 0x00, 0x01, // 0:3 number of topics
			0x00, 0x07, 't', 'o', 'p', 'i', 'c', '_', 'b', // 4:12 topic name
			0x00, 0x00, 0x00, 0x01, // 13:16 number of blocks for this topic
			0x00, 0x00, 0x00, 0x00, // 17:20 partition id
			0x00, 0x00, // 21:22 error: 0 means no error
			0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, // 23:30 offset
		}
		binary.BigEndian.PutUint64(msg[23:], 0)
		responsesB <- msg
	}()

	// TODO: Submit events to 3 different topics on 2 different brokers.
	// Need to figure out how the request format works to return the broker
	// info for those two new brokers, and how to return multiple blocks in
	// a ProduceRespose

	client, err := NewClient("client_id", []string{mockBroker.Addr()}, nil)
	if err != nil {
		t.Fatal(err)
	}

	producer, err := NewMultiProducer(client, &MultiProducerConfig{
		RequiredAcks:  WaitForLocal,
		MaxBufferTime: 1000000, // "never"
		// So that we flush once, after the 10th message.
		MaxBufferBytes: uint32((len("ABC THE MESSAGE") * 10) - 1),
	})
	defer producer.Close()

	for i := 0; i < 10; i++ {
		err = producer.SendMessage("topic_a", nil, StringEncoder("ABC THE MESSAGE"))
		if err != nil {
			t.Error(err)
		}
	}

	for i := 0; i < 10; i++ {
		err = producer.SendMessage("topic_b", nil, StringEncoder("ABC THE MESSAGE"))
		if err != nil {
			t.Error(err)
		}
	}

	select {
	case err = <-producer.Errors():
		if err != nil {
			t.Error(err)
		}
	case <-time.After(1 * time.Second):
		t.Error(fmt.Errorf("Message was never received"))
	}

	select {
	case err = <-producer.Errors():
		if err != nil {
			t.Error(err)
		}
	case <-time.After(1 * time.Second):
		t.Error(fmt.Errorf("Message was never received"))
	}

	select {
	case <-producer.Errors():
		t.Error(fmt.Errorf("too many values returned"))
	default:
	}

}
