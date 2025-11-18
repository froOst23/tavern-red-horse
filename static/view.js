const maxValue = 20;
let ws = null;
let currentActiveEvent = null;

function updateConnectionStatus(status, message = '') {
    const statusElement = document.getElementById('connectionStatus');
    statusElement.textContent = message || getStatusMessage(status);
    statusElement.className = `connection-status status-${status}`;
}

function getStatusMessage(status) {
    switch(status) {
        case 'connected': return '✓';
        case 'disconnected': return '✗';
        case 'connecting': return '⟳';
        default: return status;
    }
}

function displayActiveEvent(events) {
    const container = document.getElementById('active-event-container');

    // Находим текущее активное событие
    const activeEvent = events.find(event => event.current);

    if (activeEvent && activeEvent.title !== currentActiveEvent) {
        currentActiveEvent = activeEvent.title;

        let imageHtml = '';
        if (activeEvent.image_path) {
            const imageUrl = `/static/events/${activeEvent.image_path}`;
            imageHtml = `
                <div class="event-image">
                    <img src="${imageUrl}" alt="${activeEvent.title}" 
                         onerror="this.style.display='none'">
                </div>
            `;
        }

        container.innerHTML = `
            <div class="active-event">
                <div class="active-event-border"></div>
                ${imageHtml}
                <div class="event-content">
                    <div class="event-title">${activeEvent.title}</div>
                    ${activeEvent.description ? `<div class="event-description">${activeEvent.description}</div>` : ''}
                </div>
            </div>
        `;
    } else if (!activeEvent && currentActiveEvent !== null) {
        currentActiveEvent = null;
        container.innerHTML = `
            <div class="no-active-event">
                🎭 Активных событий нет
            </div>
        `;
    }
}

async function fetchEvents() {
    try {
        const res = await fetch('/viewer/events');
        if (!res.ok) {
            throw new Error(`HTTP error! status: ${res.status}`);
        }
        const events = await res.json();
        displayActiveEvent(events);
    } catch (error) {
        console.error('Error fetching events:', error);
        const container = document.getElementById('active-event-container');
        container.innerHTML = `
            <div class="no-active-event">
                ⚠️ Не удалось загрузить события
            </div>
        `;
    }
}

function connectWebSocket() {
    updateConnectionStatus('connecting');

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(`${protocol}//${window.location.host}/viewer/ws`);

    ws.onopen = function() {
        console.log('WebSocket connected');
        updateConnectionStatus('connected');
        fetchTeams();
        fetchEvents();
    };

    ws.onmessage = function(event) {
        const data = JSON.parse(event.data);
        console.log('WebSocket message received:', data);

        switch(data.type) {
            case 'teams_updated':
            case 'team_created':
            case 'teams_reset':
            case 'world_reset':
                console.log('Teams data changed, refreshing...');
                fetchTeams();
                break;
            case 'events_updated':
            case 'event_created':
            case 'event_deleted':
            case 'event_status_changed':
            case 'event_changed':
            case 'events_reset':
            case 'world_reset':
                console.log('Events data changed, refreshing...');
                fetchEvents();
                break;
            case 'connected':
                console.log('Connected to server');
                break;
        }
    };

    ws.onclose = function() {
        console.log('WebSocket disconnected, reconnecting in 3 seconds...');
        updateConnectionStatus('disconnected');
        setTimeout(connectWebSocket, 3000);
    };

    ws.onerror = function(error) {
        console.error('WebSocket error:', error);
        updateConnectionStatus('disconnected', 'Ошибка подключения');
    };
}

async function fetchTeams() {
    try {
        const res = await fetch('/viewer/teams');
        if (!res.ok) {
            throw new Error(`HTTP error! status: ${res.status}`);
        }
        const teams = await res.json();

        const container = document.getElementById('teams-container');
        container.innerHTML = '';

        if (!teams || teams.length === 0) {
            container.innerHTML = '<div class="team-card"><h2>Нет команд</h2></div>';
            return;
        }

        teams.forEach(t => {
            const card = document.createElement('div');
            card.className = 'team-card';

            const name = document.createElement('h2');
            name.textContent = t.name;
            name.style.textAlign = "center";
            card.appendChild(name);

            const svgNS = "http://www.w3.org/2000/svg";
            const svg = document.createElementNS(svgNS, "svg");

            svg.setAttribute("viewBox", "0 0 600 265");
            svg.setAttribute("preserveAspectRatio", "xMidYMid meet");
            svg.style.margin = "10px 0";

            const cx = 300;
            const cy = 130;
            const rx = 260;
            const ry = 100;

            const ovalPosition = (angle) => {
                const rad = (angle - 375) * Math.PI / 70;
                return {
                    x: cx + rx * Math.cos(rad),
                    y: cy + ry * Math.sin(rad)
                };
            };

            const centerGroup = document.createElementNS(svgNS, "g");

            const separator = document.createElementNS(svgNS, "text");
            separator.setAttribute("x", cx);
            separator.setAttribute("y", cy - 5);
            separator.setAttribute("text-anchor", "middle");
            separator.setAttribute("font-size", "20");
            separator.setAttribute("fill", "#5D4037");
            separator.textContent = `Жизнь ${t.health} | ${t.drunk} Опьянение`;
            centerGroup.appendChild(separator);

            svg.appendChild(centerGroup);

            for (let i = 0; i <= maxValue; i++) {
                const angle = 210 + (i / maxValue) * 120;
                const pos = ovalPosition(angle);

                const text = document.createElementNS(svgNS, "text");
                text.setAttribute("x", pos.x);
                text.setAttribute("y", pos.y + 3);
                text.setAttribute("text-anchor", "middle");
                text.setAttribute("font-size", "26");
                text.setAttribute("font-weight", "bold");
                text.setAttribute("fill", "#5D4037");
                text.textContent = i;
                svg.appendChild(text);
            }

            const healthAngle = 210 + (t.health / maxValue) * 120;
            const healthPos = ovalPosition(healthAngle);

            const healthCircle = document.createElementNS(svgNS, "circle");
            healthCircle.setAttribute("cx", healthPos.x);
            healthCircle.setAttribute("cy", healthPos.y-6);
            healthCircle.setAttribute("r", 25);
            healthCircle.setAttribute("fill", "none");
            healthCircle.setAttribute("stroke", "#DC2626");
            healthCircle.setAttribute("stroke-width", "6");
            svg.appendChild(healthCircle);

            const drunkAngle = 210 + (t.drunk / maxValue) * 120;
            const drunkPos = ovalPosition(drunkAngle);

            const drunkCircle = document.createElementNS(svgNS, "circle");
            drunkCircle.setAttribute("cx", drunkPos.x);
            drunkCircle.setAttribute("cy", drunkPos.y-6);
            drunkCircle.setAttribute("r", 25);
            drunkCircle.setAttribute("fill", "none");
            drunkCircle.setAttribute("stroke", "#2563EB");
            drunkCircle.setAttribute("stroke-width", "4");
            svg.appendChild(drunkCircle);

            if (t.health === t.drunk) {
                const warning = document.createElementNS(svgNS, "text");
                warning.setAttribute("x", cx);
                warning.setAttribute("y", 190);
                warning.setAttribute("text-anchor", "middle");
                warning.setAttribute("font-size", "16");
                warning.setAttribute("font-weight", "bold");
                warning.setAttribute("fill", "#DC2626");
                warning.textContent = "⚡ КОМАНДА ПРОИГРАЛА! ⚡";
                svg.appendChild(warning);
            }

            card.appendChild(svg);
            container.appendChild(card);
        });
    } catch (error) {
        console.error('Error fetching teams:', error);
        const container = document.getElementById('teams-container');
        container.innerHTML = '<div class="team-card"><h2>Ошибка загрузки данных</h2><p>Попробуйте обновить страницу</p></div>';
    }
}

window.onload = function() {
    connectWebSocket();
};