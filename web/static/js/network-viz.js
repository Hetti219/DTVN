// Network Topology Visualization using D3.js
export class NetworkViz {
    constructor(api, ws) {
        this.api = api;
        this.ws = ws;
        this.nodes = [];
        this.links = [];
        this.simulation = null;
        this.svg = null;
        this.g = null;
        this.zoom = null;
        this.isSupervisorMode = false;
        this._wsHandlers = [];
        this._loadTimer = null;
        this._isLoading = false;
        this._pendingLoad = false;
    }

    async render(container) {
        // Clean up previous WebSocket handlers to prevent accumulation
        this._cleanupWS();

        container.innerHTML = `
            <div class="page-header">
                <h2 class="page-title">Network Topology</h2>
                <p class="page-description">Visual representation of peer connections</p>
            </div>

            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">Network Graph <span id="network-node-count" style="font-weight: normal; color: var(--color-text-muted);"></span></h3>
                    <div class="quick-actions">
                        <button class="btn btn-sm btn-secondary" id="reset-zoom-btn">Reset Zoom</button>
                        <button class="btn btn-sm btn-secondary" id="refresh-network-btn">Refresh</button>
                    </div>
                </div>
                <div class="card-body">
                    <div id="network-graph" style="min-height: 600px; position: relative; background-color: var(--color-bg); border-radius: var(--radius-md);">
                    </div>
                </div>
            </div>

            <!-- Legend -->
            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">Legend</h3>
                </div>
                <div class="card-body">
                    <div style="display: flex; gap: 2rem; flex-wrap: wrap;">
                        <div style="display: flex; align-items: center; gap: 0.5rem;">
                            <svg width="20" height="20"><circle cx="10" cy="10" r="8" fill="#C9A84C"/></svg>
                            <span>Primary Node</span>
                        </div>
                        <div style="display: flex; align-items: center; gap: 0.5rem;">
                            <svg width="20" height="20"><circle cx="10" cy="10" r="8" fill="#4CAF82"/></svg>
                            <span>Running Replica</span>
                        </div>
                        <div style="display: flex; align-items: center; gap: 0.5rem;">
                            <svg width="20" height="20"><circle cx="10" cy="10" r="8" fill="#D4A847"/></svg>
                            <span>Starting Node</span>
                        </div>
                        <div style="display: flex; align-items: center; gap: 0.5rem;">
                            <svg width="20" height="20"><circle cx="10" cy="10" r="8" fill="#C75050"/></svg>
                            <span>Error / Stopped</span>
                        </div>
                    </div>
                </div>
            </div>
        `;

        // Setup buttons
        document.getElementById('reset-zoom-btn').addEventListener('click', () => this.resetZoom());
        document.getElementById('refresh-network-btn').addEventListener('click', () => this.refresh());

        // Initialize D3 visualization
        this.initD3();

        // Setup WebSocket listeners for live updates
        this._setupWS();

        // Load network data
        await this._loadNetworkData();
    }

    _cleanupWS() {
        for (const [evt, fn] of this._wsHandlers) {
            this.ws.off(evt, fn);
        }
        this._wsHandlers = [];
        if (this._loadTimer) {
            clearTimeout(this._loadTimer);
            this._loadTimer = null;
        }
    }

    _setupWS() {
        const handler = () => this._scheduleLoad();
        this.ws.on('node_status', handler);
        this.ws.on('cluster_status', handler);
        this._wsHandlers = [
            ['node_status', handler],
            ['cluster_status', handler],
        ];
    }

    _scheduleLoad() {
        // Debounce rapid-fire updates (e.g., 30 nodes starting sends 30+ events)
        if (this._loadTimer) clearTimeout(this._loadTimer);
        this._loadTimer = setTimeout(() => this._loadNetworkData(), 500);
    }

    refresh() {
        // Immediate refresh for user-initiated action
        if (this._loadTimer) clearTimeout(this._loadTimer);
        this._isLoading = false; // Allow immediate reload
        this._loadNetworkData();
    }

    initD3() {
        const graphContainer = document.getElementById('network-graph');
        const width = graphContainer.clientWidth || 800;
        const height = 600;

        // Create SVG
        this.svg = d3.select('#network-graph')
            .append('svg')
            .attr('width', width)
            .attr('height', height);

        // Store zoom behavior so resetZoom can reuse the same instance
        this.zoom = d3.zoom()
            .scaleExtent([0.1, 4])
            .on('zoom', (event) => {
                this.g.attr('transform', event.transform);
            });

        this.svg.call(this.zoom);

        // Create container group
        this.g = this.svg.append('g');

        // Create force simulation
        this.simulation = d3.forceSimulation()
            .force('link', d3.forceLink().id(d => d.id).distance(120))
            .force('charge', d3.forceManyBody().strength(-300))
            .force('center', d3.forceCenter(width / 2, height / 2))
            .force('collision', d3.forceCollide().radius(35));
    }

    async _loadNetworkData() {
        // Prevent concurrent loads from racing
        if (this._isLoading) {
            this._pendingLoad = true;
            return;
        }
        this._isLoading = true;
        this._pendingLoad = false;

        try {
            let loaded = false;

            // Try supervisor /nodes endpoint first
            if (!loaded) {
                try {
                    const nodes = await this.api.request('/nodes');
                    if (Array.isArray(nodes) && nodes.length > 0) {
                        this.isSupervisorMode = true;
                        this.buildSupervisorGraph(nodes);
                        loaded = true;
                    }
                } catch (e) {
                    // /nodes failed, continue to fallback
                }
            }

            // Fallback: /status endpoint (always available in supervisor, includes nodes)
            if (!loaded) {
                try {
                    const status = await this.api.request('/status');
                    if (status && Array.isArray(status.nodes) && status.nodes.length > 0) {
                        this.isSupervisorMode = true;
                        this.buildSupervisorGraph(status.nodes);
                        loaded = true;
                    }
                } catch (e) {
                    // Continue to validator fallback
                }
            }

            // Final fallback: validator mode (single node + its peers)
            if (!loaded) {
                try {
                    this.isSupervisorMode = false;
                    await this.buildValidatorGraph();
                    loaded = true;
                } catch (e) {
                    console.error('Failed to load network data:', e);
                }
            }

            if (loaded) {
                this.renderGraph();
            }
        } finally {
            this._isLoading = false;
            // If a load was requested while we were busy, schedule another
            if (this._pendingLoad) {
                this._pendingLoad = false;
                this._scheduleLoad();
            }
        }
    }

    buildSupervisorGraph(managedNodes) {
        this.nodes = [];
        this.links = [];

        // Add all managed nodes
        managedNodes.forEach(node => {
            this.nodes.push({
                id: node.id,
                label: node.id,
                isPrimary: node.is_primary || false,
                isRunning: node.status === 'running',
                isStarting: node.status === 'starting',
                isError: node.status === 'error',
                isStopped: node.status === 'stopped',
                status: node.status,
                port: node.port,
                apiPort: node.api_port
            });
        });

        // Build links: star topology with primary/bootstrap as hub
        // This reflects the actual bootstrap connection pattern
        const primary = this.nodes.find(n => n.isPrimary) || this.nodes.find(n => n.id === 'node0');
        if (primary) {
            this.nodes.forEach(node => {
                if (node.id !== primary.id && (node.isRunning || node.isStarting)) {
                    this.links.push({
                        source: primary.id,
                        target: node.id,
                    });
                }
            });
        }

        // Update node count display
        this._updateNodeCount();
    }

    async buildValidatorGraph() {
        const stats = await this.api.getStats() || {};
        const peersResponse = await this.api.getPeers() || {};
        const peersData = peersResponse.peers || [];

        this.nodes = [];
        this.links = [];

        // Add current node
        const currentId = stats.node_id || 'node0';
        this.nodes.push({
            id: currentId,
            label: currentId,
            isPrimary: stats.is_primary || false,
            isRunning: true,
            isStarting: false,
            isError: false,
            isStopped: false,
            status: 'running'
        });

        // Add peer nodes
        peersData.forEach(peer => {
            const peerId = peer.id;
            this.nodes.push({
                id: peerId,
                label: peerId.length > 12 ? peerId.substring(peerId.length - 8) : peerId,
                isPrimary: false,
                isRunning: true,
                isStarting: false,
                isError: false,
                isStopped: false,
                status: 'running'
            });

            this.links.push({
                source: currentId,
                target: peerId
            });
        });

        this._updateNodeCount();
    }

    _updateNodeCount() {
        const countEl = document.getElementById('network-node-count');
        if (!countEl) return;

        const running = this.nodes.filter(n => n.isRunning).length;
        const total = this.nodes.length;
        if (total === 0) {
            countEl.textContent = '';
        } else {
            countEl.textContent = `(${running}/${total} running)`;
        }
    }

    getNodeColor(d) {
        if (d.isError || d.isStopped) return '#C75050';
        if (d.isStarting) return '#D4A847';
        if (d.isPrimary) return '#C9A84C';
        if (d.isRunning) return '#4CAF82';
        return '#7A7F8D';
    }

    renderGraph() {
        if (!this.svg || !this.g) return;

        // Clear existing elements
        this.g.selectAll('*').remove();

        if (this.nodes.length === 0) {
            const graphContainer = document.getElementById('network-graph');
            const width = (graphContainer?.clientWidth || 800);
            this.g.append('text')
                .attr('x', width / 2)
                .attr('y', 300)
                .attr('text-anchor', 'middle')
                .attr('fill', '#7A7F8D')
                .attr('font-size', '14px')
                .text('No nodes available. Start a cluster to see the network graph.');
            return;
        }

        // Create links
        const link = this.g.append('g')
            .selectAll('line')
            .data(this.links)
            .enter().append('line')
            .attr('stroke', '#5B8DEF')
            .attr('stroke-opacity', 0.25)
            .attr('stroke-width', 1.5);

        // Create node groups
        const node = this.g.append('g')
            .selectAll('g')
            .data(this.nodes)
            .enter().append('g')
            .call(d3.drag()
                .on('start', (event, d) => this.dragStarted(event, d))
                .on('drag', (event, d) => this.dragged(event, d))
                .on('end', (event, d) => this.dragEnded(event, d)));

        // Add circles for nodes
        node.append('circle')
            .attr('r', d => d.isPrimary ? 20 : 16)
            .attr('fill', d => this.getNodeColor(d))
            .attr('stroke', d => d.isPrimary ? '#A8893B' : 'none')
            .attr('stroke-width', d => d.isPrimary ? 2 : 0);

        // Add labels
        node.append('text')
            .text(d => d.label)
            .attr('x', 0)
            .attr('y', 30)
            .attr('text-anchor', 'middle')
            .attr('fill', '#E8E9ED')
            .attr('font-size', '12px');

        // Add tooltips
        node.append('title')
            .text(d => {
                let tip = `${d.id}\nRole: ${d.isPrimary ? 'Primary' : 'Replica'}\nStatus: ${d.status}`;
                if (d.port) tip += `\nP2P Port: ${d.port}`;
                if (d.apiPort) tip += `\nAPI Port: ${d.apiPort}`;
                return tip;
            });

        // Update simulation
        this.simulation
            .nodes(this.nodes)
            .on('tick', () => {
                link
                    .attr('x1', d => d.source.x)
                    .attr('y1', d => d.source.y)
                    .attr('x2', d => d.target.x)
                    .attr('y2', d => d.target.y);

                node
                    .attr('transform', d => `translate(${d.x},${d.y})`);
            });

        this.simulation.force('link')
            .links(this.links);

        this.simulation.alpha(1).restart();
    }

    dragStarted(event, d) {
        if (!event.active) this.simulation.alphaTarget(0.3).restart();
        d.fx = d.x;
        d.fy = d.y;
    }

    dragged(event, d) {
        d.fx = event.x;
        d.fy = event.y;
    }

    dragEnded(event, d) {
        if (!event.active) this.simulation.alphaTarget(0);
        d.fx = null;
        d.fy = null;
    }

    resetZoom() {
        if (!this.svg || !this.zoom) return;

        this.svg.transition()
            .duration(750)
            .call(this.zoom.transform, d3.zoomIdentity);
    }

    handleWSEvent(event) {
        if (event.type === 'node_status' || event.type === 'cluster_status') {
            this._scheduleLoad();
        }
    }
}
