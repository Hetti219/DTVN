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
        this.currentNode = null;
        this.isSupervisorMode = false;
    }

    async render(container) {
        container.innerHTML = `
            <div class="page-header">
                <h2 class="page-title">Network Topology</h2>
                <p class="page-description">Visual representation of peer connections</p>
            </div>

            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">Network Graph</h3>
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
                            <svg width="20" height="20"><circle cx="10" cy="10" r="8" fill="#3B82F6"/></svg>
                            <span>Primary Node</span>
                        </div>
                        <div style="display: flex; align-items: center; gap: 0.5rem;">
                            <svg width="20" height="20"><circle cx="10" cy="10" r="8" fill="#10B981"/></svg>
                            <span>Running Replica</span>
                        </div>
                        <div style="display: flex; align-items: center; gap: 0.5rem;">
                            <svg width="20" height="20"><circle cx="10" cy="10" r="8" fill="#F59E0B"/></svg>
                            <span>Starting Node</span>
                        </div>
                        <div style="display: flex; align-items: center; gap: 0.5rem;">
                            <svg width="20" height="20"><circle cx="10" cy="10" r="8" fill="#DC2626"/></svg>
                            <span>Error / Stopped</span>
                        </div>
                    </div>
                </div>
            </div>
        `;

        // Setup buttons
        document.getElementById('reset-zoom-btn').addEventListener('click', () => this.resetZoom());
        document.getElementById('refresh-network-btn').addEventListener('click', () => this.loadNetworkData());

        // Initialize D3 visualization
        this.initD3();

        // Load network data
        await this.loadNetworkData();

        // Setup WebSocket listeners for live updates
        this.ws.on('node_status', () => this.loadNetworkData());
        this.ws.on('cluster_status', () => this.loadNetworkData());
    }

    initD3() {
        const graphContainer = document.getElementById('network-graph');
        const width = graphContainer.clientWidth;
        const height = 600;

        // Create SVG
        this.svg = d3.select('#network-graph')
            .append('svg')
            .attr('width', width)
            .attr('height', height);

        // Add zoom behavior
        const zoom = d3.zoom()
            .scaleExtent([0.1, 4])
            .on('zoom', (event) => {
                this.g.attr('transform', event.transform);
            });

        this.svg.call(zoom);

        // Create container group
        this.g = this.svg.append('g');

        // Create force simulation
        this.simulation = d3.forceSimulation()
            .force('link', d3.forceLink().id(d => d.id).distance(150))
            .force('charge', d3.forceManyBody().strength(-400))
            .force('center', d3.forceCenter(width / 2, height / 2))
            .force('collision', d3.forceCollide().radius(40));
    }

    async loadNetworkData() {
        try {
            // Try supervisor mode first: get all managed nodes
            const managedNodes = await this.api.request('/nodes');
            if (managedNodes && Array.isArray(managedNodes) && managedNodes.length > 0) {
                this.isSupervisorMode = true;
                this.buildSupervisorGraph(managedNodes);
            } else {
                // Fallback: single validator mode
                this.isSupervisorMode = false;
                await this.buildValidatorGraph();
            }

            this.renderGraph();
        } catch (error) {
            // Fallback to validator mode if supervisor endpoint fails
            try {
                this.isSupervisorMode = false;
                await this.buildValidatorGraph();
                this.renderGraph();
            } catch (e) {
                console.error('Failed to load network data:', e);
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

        // Build links: connect all running nodes to each other (P2P mesh)
        const runningNodes = this.nodes.filter(n => n.isRunning);
        for (let i = 0; i < runningNodes.length; i++) {
            for (let j = i + 1; j < runningNodes.length; j++) {
                this.links.push({
                    source: runningNodes[i].id,
                    target: runningNodes[j].id
                });
            }
        }
    }

    async buildValidatorGraph() {
        const stats = await this.api.getStats() || {};
        this.currentNode = stats.node_id;

        const peersResponse = await this.api.getPeers() || {};
        const peersData = peersResponse.peers || [];

        this.nodes = [];
        this.links = [];

        // Add current node
        this.nodes.push({
            id: stats.node_id || 'node0',
            label: stats.node_id || 'node0',
            isPrimary: stats.is_primary || false,
            isRunning: true,
            isStarting: false,
            isError: false,
            isStopped: false,
            status: 'running'
        });

        // Add peer nodes
        peersData.forEach(peer => {
            this.nodes.push({
                id: peer.id,
                label: peer.id.substring(peer.id.length - 8),
                isPrimary: false,
                isRunning: true,
                isStarting: false,
                isError: false,
                isStopped: false,
                status: 'running'
            });

            this.links.push({
                source: stats.node_id || 'node0',
                target: peer.id
            });
        });
    }

    getNodeColor(d) {
        if (d.isError || d.isStopped) return '#DC2626';
        if (d.isStarting) return '#F59E0B';
        if (d.isPrimary) return '#3B82F6';
        if (d.isRunning) return '#10B981';
        return '#6B7280';
    }

    renderGraph() {
        if (!this.svg || !this.g) return;

        // Clear existing elements
        this.g.selectAll('*').remove();

        if (this.nodes.length === 0) return;

        // Create links
        const link = this.g.append('g')
            .selectAll('line')
            .data(this.links)
            .enter().append('line')
            .attr('stroke', '#999')
            .attr('stroke-opacity', 0.4)
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
            .attr('stroke', d => d.isPrimary ? '#1D4ED8' : 'none')
            .attr('stroke-width', d => d.isPrimary ? 3 : 0);

        // Add labels
        node.append('text')
            .text(d => d.label)
            .attr('x', 0)
            .attr('y', 30)
            .attr('text-anchor', 'middle')
            .attr('fill', 'var(--color-text)')
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
        if (!this.svg) return;

        const graphContainer = document.getElementById('network-graph');
        const width = graphContainer.clientWidth;
        const height = 600;

        this.svg.transition()
            .duration(750)
            .call(d3.zoom().transform, d3.zoomIdentity);

        // Recenter simulation
        this.simulation.force('center', d3.forceCenter(width / 2, height / 2));
        this.simulation.alpha(0.3).restart();
    }

    handleWSEvent(event) {
        if (event.type === 'node_status' || event.type === 'cluster_status') {
            this.loadNetworkData();
        }
    }
}
