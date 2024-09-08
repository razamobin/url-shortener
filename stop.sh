#!/bin/bash

SHUTDOWN_PORT=8081

# Attempt to connect to the shutdown port
if nc -z localhost $SHUTDOWN_PORT; then
    echo "Sending shutdown signal to server..."
    nc localhost $SHUTDOWN_PORT
    echo "Shutdown signal sent. Waiting for server to stop..."
    
    # Wait for the server to stop (you might want to add a timeout here)
    while nc -z localhost 8080; do
        sleep 1
    done
    
    echo "Server stopped"
else
    echo "Server is not running or not responding on the shutdown port."
fi