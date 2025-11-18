// admin.js
// Константы
const MAX_HEALTH = 20;
const MAX_DRUNK = 20;
const MIN_VALUE = 0;
const MAX_TEAMS = 2;

// Глобальные переменные
let eventsStatus = {};
let ws = null;

// ==================== ИНИЦИАЛИЗАЦИЯ ====================
window.onload = () => {
    fetchTeams();
    fetchEvents();
    addMagicEffects();
    setupEventListeners();
    connectWebSocket();
};

function setupEventListeners() {
    const createEventBtn = document.getElementById('create-event-btn');
    if (createEventBtn) {
        createEventBtn.addEventListener('click', createNewEvent);
    }

    document.addEventListener('keydown', handleKeyboardShortcuts);
}

function handleKeyboardShortcuts(e) {
    if (e.ctrlKey || e.metaKey) {
        switch (e.key) {
            case 'Enter':
                e.preventDefault();
                const activeElement = document.activeElement;
                if (activeElement.type === 'text' || activeElement.type === 'textarea') {
                    activeElement.blur();
                }
                break;
        }
    }
}

// ==================== WEBSOCKET ====================
function connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(`${protocol}//${window.location.host}/viewer/ws`);

    ws.onopen = function () {
        console.log('WebSocket connected in admin');
    };

    ws.onmessage = function (event) {
        const data = JSON.parse(event.data);
        console.log('WebSocket message received in admin:', data);

        switch (data.type) {
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

    ws.onclose = function () {
        console.log('WebSocket disconnected in admin, reconnecting in 3 seconds...');
        setTimeout(connectWebSocket, 3000);
    };

    ws.onerror = function (error) {
        console.error('WebSocket error in admin:', error);
    };
}

// ==================== ВИЗУАЛЬНЫЕ ЭФФЕКТЫ ====================
function addMagicEffects() {
    document.querySelectorAll('.magic-btn').forEach(btn => {
        btn.addEventListener('mouseenter', function (e) {
            createParticles(e.target, 5, '#ffd700');
        });
    });

    animateCardsOnLoad();
}

function animateCardsOnLoad() {
    const cards = document.querySelectorAll('.team-card, .event-card, .magic-card');
    cards.forEach((card, index) => {
        card.style.opacity = '0';
        card.style.transform = 'translateY(20px)';
        setTimeout(() => {
            card.style.transition = 'all 0.5s ease';
            card.style.opacity = '1';
            card.style.transform = 'translateY(0)';
        }, index * 100);
    });
}

function createParticles(element, count, color) {
    for (let i = 0; i < count; i++) {
        const particle = document.createElement('div');
        particle.style.cssText = `
            position: absolute;
            width: 4px;
            height: 4px;
            background: ${color};
            border-radius: 50%;
            pointer-events: none;
            z-index: 1000;
        `;

        const rect = element.getBoundingClientRect();
        const x = rect.left + rect.width / 2;
        const y = rect.top + rect.height / 2;

        particle.style.left = x + 'px';
        particle.style.top = y + 'px';

        document.body.appendChild(particle);

        const angle = Math.random() * Math.PI * 2;
        const speed = 2 + Math.random() * 2;
        const vx = Math.cos(angle) * speed;
        const vy = Math.sin(angle) * speed;

        let opacity = 1;
        const animate = () => {
            opacity -= 0.02;
            particle.style.opacity = opacity;
            particle.style.left = parseFloat(particle.style.left) + vx + 'px';
            particle.style.top = parseFloat(particle.style.top) + vy + 'px';

            if (opacity > 0) {
                requestAnimationFrame(animate);
            } else {
                particle.remove();
            }
        };

        animate();
    }
}

// ==================== РАБОТА С КОМАНДАМИ ====================
async function fetchTeams() {
    try {
        const res = await fetch('/api/admin/teams');
        const teams = await res.json();
        renderTeams(teams);
    } catch (error) {
        console.error('Ошибка загрузки команд:', error);
        alert('Ошибка загрузки команд');
    }
}

function renderTeams(teams) {
    const container = document.getElementById('teams-container');
    container.innerHTML = '';

    teams.forEach((team, index) => {
        const card = createTeamCard(team);
        card.style.animationDelay = `${index * 0.1}s`;
        container.appendChild(card);
    });

    updateCreateButton(teams.length);
}

function createTeamCard(team) {
    const card = document.createElement('div');
    card.className = 'team-card';

    const healthAtMin = team.health <= MIN_VALUE;
    const healthAtMax = team.health >= MAX_HEALTH;
    const drunkAtMin = team.drunk <= MIN_VALUE;
    const drunkAtMax = team.drunk >= MAX_DRUNK;

    card.innerHTML = `
        <h3>${team.name}</h3>
        <input type="text" 
               class="enchanted-input" 
               id="team-name-${team.id}" 
               value="${team.name}" 
               placeholder="Название команды">
        <button class="action-btn magic-btn" onclick="updateName(${team.id})">
            ✏️ Сменить имя
        </button>
        
        <div class="stats-container">
            <div class="stat-group">
                <span class="stat-label">❤️ Здоровье</span>
                <div class="stat-value" id="health-${team.id}">${team.health}</div>
                <div class="controls">
                    <button class="control-btn" 
                            onclick="changeHealth(${team.id}, -1)" 
                            ${healthAtMin ? 'disabled' : ''}>-</button>
                    <button class="control-btn" 
                            onclick="changeHealth(${team.id}, 1)" 
                            ${healthAtMax ? 'disabled' : ''}>+</button>
                </div>
            </div>
            
            <div class="stat-group">
                <span class="stat-label">🍺 Опьянение</span>
                <div class="stat-value" id="drunk-${team.id}">${team.drunk}</div>
                <div class="controls">
                    <button class="control-btn" 
                            onclick="changeDrunk(${team.id}, -1)" 
                            ${drunkAtMin ? 'disabled' : ''}>-</button>
                    <button class="control-btn" 
                            onclick="changeDrunk(${team.id}, 1)" 
                            ${drunkAtMax ? 'disabled' : ''}>+</button>
                </div>
            </div>
        </div>
    `;

    return card;
}

function updateCreateButton(teamsCount) {
    const addBtn = document.getElementById('create-team-btn');
    const formTeam = document.getElementById('create-team');
    if (teamsCount >= MAX_TEAMS) {
        addBtn.disabled = true;
        addBtn.textContent = 'Максимум!';
        formTeam.style.display = 'none';
    } else {
        addBtn.disabled = false;
        addBtn.textContent = 'Создать';
    }
}

async function createTeam() {
    const name = document.getElementById('new-team-name').value.trim();
    if (!name) {
        alert('Введите название команды');
        return;
    }

    try {
        const res = await fetch('/api/admin/teams', {
            method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({name})
        });

        if (res.ok) {
            document.getElementById('new-team-name').value = '';
            await fetchTeams();
        } else {
            throw new Error('Ошибка сервера');
        }
    } catch (error) {
        console.error('Ошибка создания команды:', error);
        alert('Ошибка создания команды');
    }
}

async function updateName(id) {
    const newName = document.getElementById(`team-name-${id}`).value.trim();
    if (!newName) {
        alert('Введите имя команды');
        return;
    }

    try {
        const res = await fetch(`/api/admin/teams/${id}/name`, {
            method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({name: newName})
        });

        if (!res.ok) throw new Error('Ошибка сервера');
        await fetchTeams();
    } catch (error) {
        console.error('Ошибка смены имени:', error);
        alert('Ошибка смены имени команды');
    }
}

async function changeHealth(id, delta) {
    await updateTeamStat(id, 'health', delta, MAX_HEALTH);
}

async function changeDrunk(id, delta) {
    await updateTeamStat(id, 'drunk', delta, MAX_DRUNK);
}

async function updateTeamStat(id, stat, delta, maxValue) {
    const element = document.getElementById(`${stat}-${id}`);
    if (!element) return;

    const current = parseInt(element.textContent, 10);
    const newValue = current + delta;

    if (newValue < MIN_VALUE || newValue > maxValue) {
        return;
    }

    try {
        const res = await fetch(`/api/admin/teams/${id}/${stat}`, {
            method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({delta})
        });

        if (res.ok) {
            await fetchTeams();
        } else {
            throw new Error('Ошибка сервера');
        }
    } catch (error) {
        console.error(`Ошибка изменения ${stat}:`, error);
        alert(`Ошибка изменения ${stat}`);
    }
}

async function resetTeams() {
    showResetConfirmation();
}

// ==================== РАБОТА СОБЫТИЯМИ ====================
function displayCurrentEvent(events) {
    const container = document.getElementById('current-event-display');
    const nextEventBtn = document.getElementById('next-event-btn');

    // Находим текущее активное событие
    const currentEvent = events.find(event => event.current);

    if (currentEvent) {
        let imageHtml = '';
        if (currentEvent.image_path) {
            imageHtml = `
                <div class="current-event-image">
                    <img src="/static/events/${currentEvent.image_path}" alt="${currentEvent.title}">
                </div>
            `;
        }

        container.innerHTML = `
            <div class="current-event-content">
                ${imageHtml}
                <div class="current-event-title">${currentEvent.title}</div>
                ${currentEvent.description ? `<div class="current-event-description">${currentEvent.description}</div>` : ''}
            </div>
        `;
        nextEventBtn.disabled = false;
    } else {
        container.innerHTML = '<div class="no-current-event">Нет активных событий</div>';
        nextEventBtn.disabled = true;
    }
}

// Функция для переключения на следующее событие
async function nextEvent() {
    try {
        const res = await fetch('/api/admin/events/next', {
            method: 'POST'
        });

        if (res.ok) {
            const nextEvent = await res.json();
            showMagicMessage(`Переключено на: ${nextEvent.title} ✨`, 'success');
            await fetchEvents(); // Обновляем список событий
        } else if (res.status === 404) {
            showMagicMessage('Нет доступных событий для переключения!', 'error');
            await fetchEvents(); // Все равно обновляем, чтобы сбросить текущее
        } else {
            throw new Error('Ошибка сервера');
        }
    } catch (error) {
        console.error('Ошибка переключения события:', error);
        showMagicMessage('Ошибка переключения события! 💥', 'error');
    }
}

async function fetchEvents() {
    try {
        console.log('Загрузка событий...');
        const res = await fetch('/api/admin/events');
        if (!res.ok) {
            throw new Error(`HTTP error! status: ${res.status}`);
        }

        const events = await res.json();
        console.log('Получены события:', events);

        if (!events || !Array.isArray(events)) {
            console.warn('Сервер вернул не массив событий:', events);
            renderEvents([]);
            return;
        }

        renderEvents(events);
    } catch (error) {
        console.error('Ошибка загрузки событий:', error);
        alert('Ошибка загрузки событий: ' + error.message);
        renderEvents([]);
    }
}

function renderEvents(events) {
    const container = document.getElementById('events-container');
    container.innerHTML = '';

    if (!events || !Array.isArray(events)) {
        events = [];
    }

    // Отображаем текущее событие
    displayCurrentEvent(events);

    if (events.length === 0) {
        container.innerHTML = `
            <div class="event-card" style="grid-column: 1 / -1; text-align: center; display: flex; align-items: center; justify-content: center;">
                <div>
                    <h3>🎭 Событий пока нет</h3>
                    <p>Добавьте первое событие, используя форму выше</p>
                </div>
            </div>
        `;
        return;
    }

    events.forEach(event => {
        const card = createEventCard(event);
        container.appendChild(card);
    });
}

function createEventCard(event) {
    const card = document.createElement('div');
    card.className = `event-card ${event.completed ? 'completed' : ''}`;

    if (eventsStatus[event.id] === undefined) {
        eventsStatus[event.id] = event.completed || false;
    }

    // Добавляем индикатор текущего события
    const isCurrent = event.current;
    const currentIndicator = isCurrent ? '<div class="current-indicator">🎯 Текущее</div>' : '';

    card.innerHTML = `
        <button class="delete-crystal-btn" onclick="deleteEvent(${event.id})" title="Удалить событие">
            🔮
        </button>
        ${currentIndicator}
        <div class="event-title">
            <h3>${event.title}</h3>
        </div>
    `;

    card.addEventListener('click', function (e) {
        if (!e.target.closest('.delete-crystal-btn')) {
            toggleEventStatus(event.id);
        }
    });

    return card;
}

function toggleEventStatus(eventId) {
    eventsStatus[eventId] = !eventsStatus[eventId];

    const eventCard = document.querySelector(`.event-card [onclick="deleteEvent(${eventId})"]`)?.closest('.event-card');
    if (eventCard) {
        eventCard.classList.add('status-changing');

        // Переключаем класс completed
        if (eventsStatus[eventId]) {
            eventCard.classList.add('completed');
        } else {
            eventCard.classList.remove('completed');
        }

        setTimeout(() => {
            eventCard.classList.remove('status-changing');
        }, 500);
    }

    updateEventStatusOnServer(eventId, eventsStatus[eventId]);
}

async function updateEventStatusOnServer(eventId, completed) {
    try {
        const res = await fetch(`/api/admin/events/${eventId}/status`, {
            method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({completed})
        });

        if (!res.ok) {
            console.error('Ошибка обновления статуса события');
        }
    } catch (error) {
        console.error('Ошибка обновления статуса:', error);
    }
}

async function createNewEvent() {
    const title = document.getElementById('new-event-title').value.trim();
    const description = document.getElementById('new-event-description').value.trim();
    const imageFile = document.getElementById('new-event-image').files[0];

    if (!title) {
        alert('Введите название события');
        return;
    }

    try {
        // 1️⃣ Создаём событие
        const eventRes = await fetch('/api/admin/events', {
            method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({title, description})
        });
        if (!eventRes.ok) throw new Error(await eventRes.text());
        const eventData = await eventRes.json();

        // 2️⃣ Загружаем картинку
        if (imageFile) {
            await uploadEventImage(eventData.id, imageFile);
        }

        await fetchEvents();
        showMagicMessage('Событие успешно создано! ✨', 'success');

    } catch (err) {
        console.error(err);
        alert('Ошибка создания события: ' + err.message);
    }
}

async function uploadEventImage(eventId, file) {
    if (!file) return;

    const formData = new FormData();
    formData.append('image', file);

    const res = await fetch(`/api/admin/events/${eventId}/image`, {
        method: 'POST', body: formData
    });

    if (!res.ok) {
        const text = await res.text();
        throw new Error(`Ошибка загрузки изображения: ${text}`);
    }

    return await res.json();
}

// ==================== УДАЛЕНИЕ И МОДАЛЬНЫЕ ОКНА ====================
function deleteEvent(id) {
    showDeleteConfirmation(id);
}

function showResetConfirmation() {
    const modal = document.createElement('div');
    modal.id = 'magic-reset-modal';
    modal.style.cssText = `
        position: fixed;
        top: 0;
        left: 0;
        width: 100%;
        height: 100%;
        background: rgba(0,0,0,0.8);
        display: flex;
        align-items: center;
        justify-content: center;
        z-index: 1000;
        backdrop-filter: blur(5px);
    `;

    modal.innerHTML = `
        <div class="magic-confirmation-dialog" style="
            background: linear-gradient(135deg, #f4e4bc, #e8d5a8);
            padding: 30px;
            border-radius: 15px;
            border: 3px solid #c41e3a;
            text-align: center;
            max-width: 450px;
            width: 90%;
            box-shadow: 0 10px 30px rgba(0,0,0,0.5);
            position: relative;
        ">
            <div style="position: absolute; top: -20px; left: 50%; transform: translateX(-50%); 
                       background: #c41e3a; color: #ffd700; padding: 10px 20px; 
                       border-radius: 20px; border: 2px solid #ffd700; font-weight: bold;">
                🔥 Магическое Перерождение!
            </div>
            <p style="margin: 40px 0 25px 0; color: #5D4037; font-size: 16px; line-height: 1.5;">
                Вы действительно хотите переродить весь мир?<br>
                <strong>Это сбросит здоровье и опьянение всех команд!</strong><br>
                <em style="font-size: 14px; color: #8B4513;">Это действие необратимо!</em>
            </p>
            <div style="display: flex; gap: 15px; justify-content: center;">
                <button id="magic-reset-cancel-btn" 
                        style="padding: 12px 25px; background: linear-gradient(135deg, #8B4513, #a0522d); 
                               color: #ffd700; border: 2px solid #ffd700; border-radius: 8px; 
                               cursor: pointer; font-family: 'MedievalSharp', cursive; font-weight: bold;">
                    Отмена
                </button>
                <button id="magic-reset-confirm-btn" 
                        style="padding: 12px 25px; background: linear-gradient(135deg, #c41e3a, #8B0000); 
                               color: white; border: 2px solid #ff6b6b; border-radius: 8px; 
                               cursor: pointer; font-family: 'MedievalSharp', cursive; font-weight: bold;">
                    Переродить Мир!
                </button>
            </div>
        </div>
    `;

    document.body.appendChild(modal);

    const cancelBtn = document.getElementById('magic-reset-cancel-btn');
    const confirmBtn = document.getElementById('magic-reset-confirm-btn');

    cancelBtn.addEventListener('click', closeResetModal);
    confirmBtn.addEventListener('click', confirmReset);

    modal.addEventListener('click', function (e) {
        if (e.target === modal) {
            closeResetModal();
        }
    });

    document.addEventListener('keydown', function escapeHandler(e) {
        if (e.key === 'Escape') {
            closeResetModal();
            document.removeEventListener('keydown', escapeHandler);
        }
    });
}

async function confirmReset() {
    try {
        const res = await fetch('/api/admin/teams/reset', {
            method: 'POST'
        });

        if (res.ok) {
            closeResetModal();
            showMagicMessage('Мир успешно перерождён! 🌍✨', 'success');
            await fetchTeams();
        } else {
            throw new Error('Ошибка сервера');
        }
    } catch (error) {
        console.error('Ошибка сброса команд:', error);
        closeResetModal();
        showMagicMessage('Ошибка перерождения мира! 💥', 'error');
    }
}

function closeResetModal() {
    const modal = document.getElementById('magic-reset-modal');
    if (modal) {
        modal.style.opacity = '0';
        modal.style.transition = 'opacity 0.3s ease';
        setTimeout(() => {
            modal.remove();
        }, 300);
    }
}

async function confirmDelete(id) {
    try {
        const res = await fetch(`/api/admin/events/${id}`, {
            method: 'DELETE'
        });

        if (res.ok) {
            closeModal();
            showMagicMessage('Событие успешно уничтожено! ✨', 'success');
            await fetchEvents();
        } else {
            throw new Error('Ошибка сервера');
        }
    } catch (error) {
        console.error('Ошибка удаления события:', error);
        closeModal();
        showMagicMessage('Ошибка удаления события! 💥', 'error');
    }
}

function showDeleteConfirmation(eventId) {
    const modal = document.createElement('div');
    modal.id = 'magic-delete-modal';
    modal.style.cssText = `
        position: fixed;
        top: 0;
        left: 0;
        width: 100%;
        height: 100%;
        background: rgba(0,0,0,0.8);
        display: flex;
        align-items: center;
        justify-content: center;
        z-index: 1000;
        backdrop-filter: blur(5px);
    `;

    modal.innerHTML = `
        <div class="magic-confirmation-dialog" style="
            background: linear-gradient(135deg, #f4e4bc, #e8d5a8);
            padding: 30px;
            border-radius: 15px;
            border: 3px solid #c41e3a;
            text-align: center;
            max-width: 400px;
            width: 90%;
            box-shadow: 0 10px 30px rgba(0,0,0,0.5);
            position: relative;
        ">
            <div style="position: absolute; top: -20px; left: 50%; transform: translateX(-50%); 
                       background: #c41e3a; color: #ffd700; padding: 10px 20px; 
                       border-radius: 20px; border: 2px solid #ffd700; font-weight: bold;">
                Магическое предупреждение!
            </div>
            <p style="margin: 40px 0 25px 0; color: #5D4037; font-size: 16px; line-height: 1.5;">
                Вы действительно хотите уничтожить это событие?<br>
                <strong>Это действие необратимо!</strong>
            </p>
            <div style="display: flex; gap: 15px; justify-content: center;">
                <button id="magic-cancel-btn" 
                        style="padding: 12px 25px; background: linear-gradient(135deg, #8B4513, #a0522d); 
                               color: #ffd700; border: 2px solid #ffd700; border-radius: 8px; 
                               cursor: pointer; font-family: 'MedievalSharp', cursive; font-weight: bold;">
                    Отмена
                </button>
                <button id="magic-confirm-btn" 
                        style="padding: 12px 25px; background: linear-gradient(135deg, #c41e3a, #8B0000); 
                               color: white; border: 2px solid #ff6b6b; border-radius: 8px; 
                               cursor: pointer; font-family: 'MedievalSharp', cursive; font-weight: bold;">
                    Уничтожить!
                </button>
            </div>
        </div>
    `;

    document.body.appendChild(modal);

    const cancelBtn = document.getElementById('magic-cancel-btn');
    const confirmBtn = document.getElementById('magic-confirm-btn');

    cancelBtn.addEventListener('click', closeModal);
    confirmBtn.addEventListener('click', () => confirmDelete(eventId));

    modal.addEventListener('click', function (e) {
        if (e.target === modal) {
            closeModal();
        }
    });

    document.addEventListener('keydown', function escapeHandler(e) {
        if (e.key === 'Escape') {
            closeModal();
            document.removeEventListener('keydown', escapeHandler);
        }
    });
}

function closeModal() {
    const modal = document.getElementById('magic-delete-modal');
    if (modal) {
        modal.style.opacity = '0';
        modal.style.transition = 'opacity 0.3s ease';
        setTimeout(() => {
            modal.remove();
        }, 300);
    }
}

// ==================== УТИЛИТЫ ====================
function showMagicMessage(text, type) {
    const existingMessage = document.querySelector('.magic-message');
    if (existingMessage) {
        existingMessage.remove();
    }

    const message = document.createElement('div');
    message.className = 'magic-message';
    message.style.cssText = `
        position: fixed;
        top: 20px;
        right: 20px;
        padding: 15px 25px;
        background: ${type === 'success' ? 'linear-gradient(135deg, #4caf50, #388e3c)' : 'linear-gradient(135deg, #f44336, #d32f2f)'};
        color: white;
        border-radius: 8px;
        box-shadow: 0 5px 15px rgba(0,0,0,0.3);
        z-index: 1001;
        border: 2px solid #ffd700;
        font-family: 'MedievalSharp', cursive;
        font-weight: bold;
        transform: translateX(100%);
        transition: transform 0.3s ease;
    `;

    message.innerHTML = `
        <span style="margin-right: 10px; font-size: 18px;">${type === 'success' ? '✨' : '💥'}</span>
        ${text}
    `;

    document.body.appendChild(message);

    setTimeout(() => {
        message.style.transform = 'translateX(0)';
    }, 10);

    setTimeout(() => {
        message.style.transform = 'translateX(100%)';
        setTimeout(() => {
            if (message.parentElement) {
                message.remove();
            }
        }, 300);
    }, 3000);
}

// Добавляем CSS стили для анимаций
if (!document.querySelector('#magic-styles')) {
    const style = document.createElement('style');
    style.id = 'magic-styles';
    style.textContent = `
        .magic-confirmation-dialog {
            animation: dialogAppear 0.3s ease-out;
        }
        
        @keyframes dialogAppear {
            from {
                opacity: 0;
                transform: scale(0.8) translateY(-20px);
            }
            to {
                opacity: 1;
                transform: scale(1) translateY(0);
            }
        }
        
        #magic-cancel-btn:hover, #magic-confirm-btn:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 8px rgba(0,0,0,0.3);
            transition: all 0.2s ease;
        }
        
        #magic-cancel-btn:active, #magic-confirm-btn:active {
            transform: translateY(0);
        }

        @keyframes slideIn {
            from { transform: translateX(100%); opacity: 0; }
            to { transform: translateX(0); opacity: 1; }
        }
        
        @keyframes slideOut {
            from { transform: translateX(0); opacity: 1; }
            to { transform: translateX(100%); opacity: 0; }
        }
    `;
    document.head.appendChild(style);
}