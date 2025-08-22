package main

import (
	"auth-service/pkg/pb"
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Can not connect: %v", err)
	}
	fmt.Println("Client connected")
	defer conn.Close()

	c := pb.NewAuthServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	fmt.Println("Creating new account")
	r, err := c.Register(ctx, &pb.RegisterRequest{
		Username: "admin2",
		Password: "admin",
		Role:     "buyer",
	})
	fmt.Println("Created account")
	if err != nil {
		log.Fatalf("Can not register: %v", err)
	}

	log.Println("Register result:", r)
}
