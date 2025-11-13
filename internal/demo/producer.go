package demo

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// CreateKafkaProducerDeployment creates a Kubernetes Deployment for continuous Kafka message production
func CreateKafkaProducerDeployment(
	ctx context.Context,
	client kubernetes.Interface,
	namespace, broker, topic, username, password string,
	intervalSeconds int,
) error {
	deploymentName := "kafka-demo-producer"

	// Check if deployment already exists
	_, err := client.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err == nil {
		fmt.Printf("ℹ️  Kafka producer deployment '%s' already exists, skipping creation\n", deploymentName)
		return nil
	}

	// Create deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
			Labels: map[string]string{
				"app": "kafka-demo-producer",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "kafka-demo-producer",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "kafka-demo-producer",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "producer",
							Image: "python:3.11-slim",
							Command: []string{
								"sh",
								"-c",
								`pip install --quiet --no-cache-dir kafka-python && \
python3 -u << 'EOFPYTHON'
import json
import os
import time
import signal
import sys
from datetime import datetime
from uuid import uuid4

try:
    from kafka import KafkaProducer
    from kafka.errors import KafkaError
except ImportError:
    print("ERROR: kafka-python library not found", file=sys.stderr)
    sys.exit(1)

def create_producer(broker, username, password):
    return KafkaProducer(
        bootstrap_servers=[broker],
        security_protocol='SASL_PLAINTEXT',
        sasl_mechanism='PLAIN',
        sasl_plain_username=username,
        sasl_plain_password=password,
        value_serializer=lambda v: json.dumps(v).encode('utf-8'),
        key_serializer=lambda k: k.encode('utf-8') if k else None,
        acks='all',
        retries=3,
        max_in_flight_requests_per_connection=1,
    )

def produce_messages(producer, topic, interval_seconds):
    event_types = ["page_view", "click", "purchase", "signup", "login"]
    sources = ["web", "mobile", "api"]
    count = 0
    
    print(f"📨 Starting continuous producer for topic '{topic}' (interval: {interval_seconds}s)...")
    
    try:
        while True:
            event = {
                "event_id": str(uuid4()),
                "type": event_types[count % len(event_types)],
                "source": sources[count % len(sources)],
                "created_at": datetime.utcnow().isoformat() + "Z",
            }
            
            future = producer.send(topic, key=event["event_id"], value=event)
            
            try:
                record_metadata = future.get(timeout=10)
                count += 1
                if count % 10 == 0:
                    print(f"   Produced {count} messages... (partition: {record_metadata.partition}, offset: {record_metadata.offset})")
            except KafkaError as e:
                print(f"⚠️  Failed to produce message: {e}")
            
            time.sleep(interval_seconds)
            
    except KeyboardInterrupt:
        print("\n🛑 Received shutdown signal, stopping producer...")
    finally:
        producer.close()
        print("✅ Producer closed")

def main():
    broker = os.getenv("KAFKA_BROKER", "kafka.glassflow.svc.cluster.local:9092")
    topic = os.getenv("KAFKA_TOPIC", "demo-events")
    username = os.getenv("KAFKA_USERNAME", "user1")
    password = os.getenv("KAFKA_PASSWORD", "")
    interval_seconds = int(os.getenv("PRODUCE_INTERVAL_SECONDS", "1"))
    
    if not password:
        print("ERROR: KAFKA_PASSWORD environment variable is required", file=sys.stderr)
        sys.exit(1)
    
    print(f"Kafka Broker: {broker}")
    print(f"Topic: {topic}")
    print(f"Username: {username}")
    print(f"Interval: {interval_seconds}s")
    
    producer = create_producer(broker, username, password)
    
    signal.signal(signal.SIGTERM, lambda s, f: sys.exit(0))
    signal.signal(signal.SIGINT, lambda s, f: sys.exit(0))
    
    produce_messages(producer, topic, interval_seconds)

if __name__ == "__main__":
    main()
EOFPYTHON`,
							},
							Env: []corev1.EnvVar{
								{
									Name:  "KAFKA_BROKER",
									Value: broker,
								},
								{
									Name:  "KAFKA_TOPIC",
									Value: topic,
								},
								{
									Name:  "KAFKA_USERNAME",
									Value: username,
								},
								{
									Name:  "KAFKA_PASSWORD",
									Value: password,
								},
								{
									Name:  "PRODUCE_INTERVAL_SECONDS",
									Value: fmt.Sprintf("%d", intervalSeconds),
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: func() corev1.ResourceList {
									cpu, _ := resource.ParseQuantity("50m")
									mem, _ := resource.ParseQuantity("128Mi")
									return corev1.ResourceList{
										corev1.ResourceCPU:    cpu,
										corev1.ResourceMemory: mem,
									}
								}(),
								Limits: func() corev1.ResourceList {
									cpu, _ := resource.ParseQuantity("200m")
									mem, _ := resource.ParseQuantity("256Mi")
									return corev1.ResourceList{
										corev1.ResourceCPU:    cpu,
										corev1.ResourceMemory: mem,
									}
								}(),
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyAlways,
				},
			},
		},
	}

	_, err = client.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create producer deployment: %w", err)
	}

	fmt.Printf("✅ Created Kafka producer deployment '%s' (running continuously)\n", deploymentName)
	return nil
}

func int32Ptr(i int32) *int32 {
	return &i
}
