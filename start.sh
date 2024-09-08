#!/bin/bash

# Check if the server is already running
if nc -z localhost 8080; then
    echo "Server is already running. Use stop.sh to stop it first."
    exit 1
fi

# Start the server
go run main.go &

echo "Server started"