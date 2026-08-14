package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"escortcrm/internal/httpapi"
	"escortcrm/internal/service"
	"escortcrm/internal/table"
)

func main() {
	address := flag.String("addr", ":8080", "HTTP listen address")
	dataDirectory := flag.String("data", "./data", "CSV data directory")
	flag.Parse()

	store, err := table.Open(*dataDirectory)
	if err != nil {
		log.Fatal(err)
	}
	customerService := service.New(store, service.SystemClock{}, service.RandomIDs{})
	server := &http.Server{
		Addr:              *address,
		Handler:           httpapi.New(customerService),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("medical escort CRM listening on %s", *address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
