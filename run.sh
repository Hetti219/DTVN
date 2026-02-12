#!/bin/bash
# Load environment variables from .env and run the application
# Usage: ./run.sh supervisor
#        ./run.sh validator <validator-args>

if [ ! -f .env ]; then
    echo "Error: .env file not found"
    exit 1
fi

# Load .env variables
set -a
source .env
set +a

# Run the requested binary
case "$1" in
    supervisor)
        exec ./bin/supervisor "${@:2}"
        ;;
    validator)
        exec ./bin/validator "${@:2}"
        ;;
    simulator)
        exec ./bin/simulator "${@:2}"
        ;;
    *)
        echo "Usage: $0 {supervisor|validator|simulator} [args...]"
        exit 1
        ;;
esac
