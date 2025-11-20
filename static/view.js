const maxValue = 40;
let ws = null;

function renderTeamFlask(health, drunk, options = {}) {
    const MAX = 40;

    // clamp
    health = Math.max(0, Math.min(MAX, health));
    drunk = Math.max(0, Math.min(MAX, drunk));

    const {
        scale = 3,
        svgWidth = 150,
        svgHeight = 700
    } = options;

    const healthPercent = health / MAX;
    const drunkPercent = drunk / MAX;

    const H = 300 * scale + 85;
    const baseY = 320 * scale + 85;

    const healthHeight = healthPercent * H;
    const drunkHeight = drunkPercent * H;

    // Расчет размеров с учетом масштаба
    const flaskX = 45 * scale;
    const flaskY = 15 * scale;
    const flaskWidth = 90 * scale;
    const flaskHeight = 350 * scale + 85;
    const borderRadius = 40 * scale;
    const strokeWidth = 5 * scale;

    const template = document.createElement("div");

    template.innerHTML = `
        <svg viewBox="0 0 ${svgWidth} ${svgHeight}" xmlns="http://www.w3.org/2000/svg" class="team-flask-svg">
        
            <defs>
                <clipPath id="flask-clip">
                    <rect x="${flaskX}" y="${flaskY}" width="${flaskWidth}" height="${flaskHeight}" rx="${borderRadius}" ry="${borderRadius}"></rect>
                </clipPath>
        
                <linearGradient id="glassGradient" x1="0%" y1="0%" x2="100%" y2="0%">
                    <stop offset="0%" stop-color="rgba(255,255,255,0.35)" />
                    <stop offset="40%" stop-color="rgba(255,255,255,0.05)" />
                    <stop offset="100%" stop-color="rgba(255,255,255,0.35)" />
                </linearGradient>
            </defs>
        
            <g clip-path="url(#flask-clip)">
                <!-- Желтый фон (на всю высоту) -->
                <rect class="beer-liquid-2"
                      x="${flaskX}" 
                      y="${baseY}" 
                      width="${flaskWidth}" 
                      height="${320 * scale}"
                      fill="#ffcf40"/>
                      
                <!-- КРАСНАЯ ЖИДКОСТЬ -->
                <rect class="health-liquid"
                      x="${flaskX}" 
                      y="${baseY - healthHeight}" 
                      width="${flaskWidth}" 
                      height="${healthHeight}"
                      fill="#c41e3a"/>

                <!-- ЖЕЛТАЯ ЖИДКОСТЬ -->
                <rect class="beer-liquid"
                      x="${flaskX}" 
                      y="${baseY - drunkHeight}" 
                      width="${flaskWidth}" 
                      height="${drunkHeight}"
                      fill="#ffcf40"/>
            </g>

            <!-- КОНТУР КОЛБЫ -->
            <rect x="${flaskX}" y="${flaskY}" width="${flaskWidth}" height="${flaskHeight}" rx="${borderRadius}" ry="${borderRadius}"
                  fill="url(#glassGradient)" stroke="#e6e6e6" stroke-width="${strokeWidth}"/>

            <!-- ЛИНЕЙКА СЛЕВА -->
            <g stroke="#ffffffaa" stroke-width="${2 * scale}">
                ${Array.from({ length: MAX + 1 }).map((_, i) => {
        const y = baseY - (i * (H / MAX));
        // Рисуем риски слева от колбы
        return `<line x1="${5 * scale}" y1="${y}" x2="${25 * scale}" y2="${y}"/>`;
    }).join('')}
            </g>

            <!-- ЦИФРЫ НАПРОТИВ РИСОК СЛЕВА -->
            <g fill="#ffffffcc" font-size="${10 * scale}" font-family="Georgia" text-anchor="middle">
                ${Array.from({ length: MAX + 1 }).map((_, i) => {
        const y = baseY + (3 * scale) - (i * (H / MAX));
        // Цифры слева, напротив рисок
        return `<text x="${0.1 * scale -5}" y="${y}">${i}</text>`;
    }).join('')}
            </g>

        </svg>
    `;

    const svg = template.firstElementChild;
    if (health === drunk) {
        svg.classList.add("team-flask-lost");
    }
    return svg;
}

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
    const imgContainer = document.getElementById('active-event-image');
    const descContainer = document.getElementById('active-event-description');

    imgContainer.innerHTML = '';
    descContainer.innerHTML = '';

    /**
     * @typedef {Object} Event
     * @property {number} id
     * @property {string} title
     * @property {string} description
     * @property {string} type
     * @property {string} difficult
     * @property {boolean} current
     * @property {boolean} init
     * @property {boolean} used
     * @property {string} requirement
     * @property {string} victory_effect
     * @property {string} defeat_effect
     * @property {string} image_path
     * @property {string} created_at
     */

    /** @type {Event|undefined} */
    const activeEvent = events.find(e => e.current);

    if (!activeEvent) {
        descContainer.innerHTML = `<div class="no-event-message">🎭 Активных событий нет</div>`;
        return;
    }

    if (activeEvent.image_path) {
        imgContainer.innerHTML = `
            <div class="event-image-large">
                <img src="/static/events/${activeEvent.image_path}" alt="${activeEvent.title}">
            </div>
        `;
    }

    descContainer.innerHTML = `
        <div class="event-description-bottom">
            <div class="event-type">
                <span>${activeEvent.difficult}</span>
                <span>${activeEvent.type}</span>
            </div>
            <div class="event-title-bottom">${activeEvent.title}</div>
            <div class="event-description-text">${activeEvent.description || ''}</div>
        </div>
    `;
}

async function fetchEvents() {
    try {
        const res = await fetch('/viewer/events');
        if (!res.ok) {
            throw new Error(`HTTP error! status: ${res.status}`);
        }

        /**
         * @typedef {Object} Event
         * @property {number} id
         * @property {string} title
         * @property {string} description
         * @property {string} type
         * @property {string} difficult
         * @property {boolean} current
         * @property {boolean} init
         * @property {boolean} used
         * @property {string} requirement
         * @property {string} victory_effect
         * @property {string} defeat_effect
         * @property {string} image_path
         * @property {string} created_at
         */

        /** @type {Event|undefined} */
        const events = await res.json();
        console.log('Получены события:', events);
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
        if (!res.ok) throw new Error(`HTTP error! status: ${res.status}`);
        const teams = await res.json();

        const container = document.getElementById('teams-container');
        const tplTeam = document.getElementById('team-card-template');

        const left = document.getElementById('teams-left');
        const right = document.getElementById('teams-right');

        left.innerHTML = '';
        right.innerHTML = '';

        if (!teams?.length) {
            const emptyCard = tplTeam.content.cloneNode(true);
            emptyCard.querySelector('.team-name').textContent = 'Нет команд';
            container.appendChild(emptyCard);
            return;
        }


        teams.forEach((t, i) => {
            const card = tplTeam.content.cloneNode(true);
            card.querySelector('.team-name').textContent = t.name;

            // Высокие колбы на всю высоту
            const flaskSvg = renderTeamFlask(t.health, t.drunk, {
                scale: 0.8,
                svgWidth: 160,
                svgHeight: 380
            });

            card.querySelector('.team-svg').appendChild(flaskSvg);

            if (i % 2 === 0) left.appendChild(card);
            else right.appendChild(card);
        });

    } catch (error) {
        console.error('Error fetching teams:', error);
        const container = document.getElementById('teams-container');
        const tplTeam = document.getElementById('team-card-template');
        const errorCard = tplTeam.content.cloneNode(true);

        errorCard.querySelector('.team-name').textContent = 'Ошибка загрузки';
        container.innerHTML = '';
        container.appendChild(errorCard);
    }
}

function renderGameScore(health, drunk) {
    const svgNS = "http://www.w3.org/2000/svg";

    const svg = document.createElementNS(svgNS, "svg");
    svg.setAttribute("viewBox", "0 0 600 265");
    svg.setAttribute("preserveAspectRatio", "xMidYMid meet");
    svg.style.margin = "10px 0";

    const cx = 300, cy = 130, rx = 260, ry = 100;
    const ovalPosition = angle => {
        const rad = (angle - 375) * Math.PI / 70;
        return { x: cx + rx * Math.cos(rad), y: cy + ry * Math.sin(rad) };
    };

    const centerGroup = document.createElementNS(svgNS, "g");

    const separator = document.createElementNS(svgNS, "text");
    separator.setAttribute("x", cx);
    separator.setAttribute("y", cy - 5);
    separator.setAttribute("text-anchor", "middle");
    separator.setAttribute("font-size", "20");
    separator.setAttribute("fill", "#5D4037");
    separator.textContent = `Здоровье ${health} | ${drunk} Опьянение`;
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

    const healthAngle = 210 + (health / maxValue) * 120;
    const hp = ovalPosition(healthAngle);

    const healthCircle = document.createElementNS(svgNS, "circle");
    healthCircle.setAttribute("cx", hp.x);
    healthCircle.setAttribute("cy", hp.y - 6);
    healthCircle.setAttribute("r", 25);
    healthCircle.setAttribute("fill", "none");
    healthCircle.setAttribute("stroke", "#DC2626");
    healthCircle.setAttribute("stroke-width", "6");
    svg.appendChild(healthCircle);

    const drunkAngle = 210 + (drunk / maxValue) * 120;
    const dp = ovalPosition(drunkAngle);

    const drunkCircle = document.createElementNS(svgNS, "circle");
    drunkCircle.setAttribute("cx", dp.x);
    drunkCircle.setAttribute("cy", dp.y - 6);
    drunkCircle.setAttribute("r", 25);
    drunkCircle.setAttribute("fill", "none");
    drunkCircle.setAttribute("stroke", "#2563EB");
    drunkCircle.setAttribute("stroke-width", "4");
    svg.appendChild(drunkCircle);

    if (health === drunk) {
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

    return svg;
}

window.onload = function() {
    connectWebSocket();
};