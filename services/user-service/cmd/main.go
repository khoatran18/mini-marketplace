package main

import (
	"fmt"
	"log"
	"net"
	"user-service/internal/config"
	"user-service/internal/repository"
	"user-service/internal/server"
	"user-service/internal/service"
	userpb "user-service/pkg/pb"

	"github.com/lpernett/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	godotenv.Load(".env")

	serviceConfig, err := config.NewServiceConfig()
	if err != nil {
		log.Fatal("Error NewServiceConfig", err.Error())
	}
	fmt.Println("Create ServiceConfig successfully!")

	_, err = config.NewEnvConfig()
	if err != nil {
		log.Fatal("Error NewEnvConfig", err.Error())
	}
	fmt.Println("Create EnvConfig successfully!")

	// err = serviceConfig.PostgresDB.AutoMigrate(&dto.Product{})
	if err != nil {
		log.Fatalf("Can not migrate database: %v", err)
	} else {
		fmt.Println("Migration successfully!")
	}

	userRepo := repository.NewUserRepository(serviceConfig.PostgresDB)
	userService := service.NewUserService(userRepo, serviceConfig.ZapLogger)

	lis, err := net.Listen("tcp", ":50053")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	userpb.RegisterUserServiceServer(s, &server.UserServer{
		UserService: userService,
		ZapLogger:   serviceConfig.ZapLogger,
	})
	log.Printf("Product Server Listen at %v", lis.Addr())

	reflection.Register(s)

	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

}
