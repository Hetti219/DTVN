// WebSocket Client for real-time updates
export class WebSocketClient {
    constructor(baseURL) {
        this.baseURL = baseURL;
        this.ws = null;
        this.reconnectInterval = 3000;
        this.reconnectTimer = null;
        this.listeners = {};
        this.isConnected = false;
    }

    connect() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsURL = `${protocol}//${window.location.host}/ws`;

        console.log(`Connecting to WebSocket: ${wsURL}`);

        try {
            this.ws = new WebSocket(wsURL);

            this.ws.onopen = () => {
                console.log('WebSocket connected');
                this.isConnected = true;
                this.emit('connected');

                // Clear reconnect timer
                if (this.reconnectTimer) {
                    clearTimeout(this.reconnectTimer);
                    this.reconnectTimer = null;
                }
            };

            this.ws.onmessage = (event) => {
                try {
                    const message = JSON.parse(event.data);
                    console.log('WebSocket message received:', message);
                    this.emit('message', message);

                    // Emit specific event type
                    if (message.type) {
                        this.emit(message.type, message);
                    }
                } catch (error) {
                    console.error('Failed to parse WebSocket message:', error);
                }
            };

            this.ws.onerror = (error) => {
                console.error('WebSocket error:', error);
                this.emit('error', error);
            };

            this.ws.onclose = () => {
                console.log('WebSocket disconnected');
                this.isConnected = false;
                this.emit('disconnected');

                // Attempt reconnection
                this.scheduleReconnect();
            };
        } catch (error) {
            console.error('Failed to create WebSocket:', error);
            this.scheduleReconnect();
        }
    }

    scheduleReconnect() {
        if (this.reconnectTimer) return;

        console.log(`Reconnecting in ${this.reconnectInterval}ms...`);
        this.reconnectTimer = setTimeout(() => {
            this.reconnectTimer = null;
            this.connect();
        }, this.reconnectInterval);
    }

    send(data) {
        if (!this.isConnected || !this.ws) {
            console.warn('WebSocket not connected, cannot send message');
            return false;
        }

        try {
            this.ws.send(JSON.stringify(data));
            return true;
        } catch (error) {
            console.error('Failed to send WebSocket message:', error);
            return false;
        }
    }

    on(eventType, callback) {
        if (!this.listeners[eventType]) {
            this.listeners[eventType] = [];
        }
        this.listeners[eventType].push(callback);
    }

    off(eventType, callback) {
        if (!this.listeners[eventType]) return;

        this.listeners[eventType] = this.listeners[eventType].filter(
            cb => cb !== callback
        );
    }

    emit(eventType, data) {
        if (!this.listeners[eventType]) return;

        this.listeners[eventType].forEach(callback => {
            try {
                callback(data);
            } catch (error) {
                console.error(`Error in ${eventType} listener:`, error);
            }
        });
    }

    close() {
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = null;
        }

        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }

        this.isConnected = false;
    }
}
