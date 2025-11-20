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
                console.log('Events data changed, refreshing...');
                fetchEvents();
                break;
            case 'events_updated':
            case 'event_created':
            case 'event_deleted':
            case 'event_status_changed':
            case 'event_changed':
            case 'events_reset':
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
                <div class="current-event-description"><b>${currentEvent.type}</b></div>
                ${currentEvent.description ? `<div class="current-event-description">${currentEvent.description}</div>` : ''}
                <div class="current-event-description">
                    <span><b>Победа:</b> ${currentEvent.victory_effect}</span>
                </div>
                <div class="current-event-description">
                    <span><b>Поражение:</b> ${currentEvent.defeat_effect}</span>
                </div>
                <div class="current-event-description">
                    <span><b>Зависимости:</b> ${currentEvent.requirement}</span>
                </div>
                
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
            showNotification(`Дальше: ${nextEvent.title}`, 'success');
            await fetchEvents(); // Обновляем список событий
        } else if (res.status === 404) {
            showNotification('Нет доступных событий для переключения!', 'error');
            await fetchEvents(); // Все равно обновляем, чтобы сбросить текущее
        } else {
            throw new Error('Ошибка сервера');
        }
    } catch (error) {
        console.error('Ошибка переключения события:', error);
        showNotification('Ошибка переключения события! 💥', 'error');
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
    card.className = `event-card ${event.used ? 'completed' : ''}`;

    if (eventsStatus[event.id] === undefined) {
        eventsStatus[event.id] = event.completed || false;
    }

    // Добавляем индикатор текущего события
    const isCurrent = event.current;
    const isInit = event.init;

    const currentIndicator = isCurrent ? '<div class="current-indicator">Текущая</div>' : '';
    const currentInitIndicator = isInit ? '<div class="current-init-indicator">Начальная</div>' : '';

    card.innerHTML = `
        <button class="delete-crystal-btn" onclick="deleteEvent(${event.id})" title="Удалить событие">
            🔮
        </button>
        ${currentInitIndicator}
        ${currentIndicator}
        <div class="event-title">
            <h3>${event.title}</h3>
            <div  class="event-info">
                ${event.type} • ${event.difficult}
            </div>
        </div>
    `;

    card.addEventListener('click', function (e) {
        if (!e.target.closest('.delete-crystal-btn')) {
            updateEventCard(event.id, event);
        }
    });

    return card;
}

function updateEventCard(eventId, event) {
    showEditEventModal(event);
}

function showEditEventModal(event) {
    const modal = document.createElement('div');
    modal.id = 'edit-event-modal';
    modal.className = 'edit-event-modal';
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
        overflow-y: auto;
        padding: 10px;
    `;

    modal.innerHTML = `
        <div class="magic-edit-dialog" style="
            background: linear-gradient(135deg, #f4e4bc, #e8d5a8);
            padding: 30px;
            border-radius: 15px;
            border: 3px solid #1976d2;
            max-width: 600px;
            width: 90%;
            max-height: 90vh;
            overflow-y: auto;
            box-shadow: 0 10px 30px rgba(0,0,0,0.5);
            position: relative;
        ">
            <div style="position: absolute; top: 5px; left: 50%; transform: translateX(-50%); 
                       background: #1976d2; color: #ffd700; padding: 10px 20px; 
                       border-radius: 20px; border: 2px solid #ffd700; font-weight: bold;">
                Редактировать
            </div>
            
            <div style="margin: 40px 0 20px 0;">
                <div class="form-group">
                    <label style="display: block; margin-bottom: 8px; color: #5D4037; font-weight: bold;">
                        Название события
                    </label>
                    <input type="text" id="edit-event-title" class="enchanted-input" 
                           value="${escapeHtml(event.title)}" placeholder="Название события...">
                </div>

                <div class="form-group">
                    <label style="display: block; margin-bottom: 8px; color: #5D4037; font-weight: bold;">
                        Описание события
                    </label>
                    <textarea id="edit-event-description" class="enchanted-input event-description" 
                              placeholder="Описание события..." rows="3">${escapeHtml(event.description)}</textarea>
                </div>

                <div class="form-group">
                    <label style="display: block; margin-bottom: 8px; color: #5D4037; font-weight: bold;">
                        Тип события
                    </label>
                    <input type="text" id="edit-event-type" class="enchanted-input" 
                           value="${escapeHtml(event.type)}" placeholder="Тип события...">
                </div>

                <div class="form-group">
                    <label style="display: block; margin-bottom: 8px; color: #5D4037; font-weight: bold;">
                        Сложность
                    </label>
                    <input type="text" id="edit-event-difficulty" class="enchanted-input" 
                           value="${escapeHtml(event.difficult)}" placeholder="Сложность события...">
                </div>

                <div class="form-group">
                    <label style="display: block; margin-bottom: 8px; color: #5D4037; font-weight: bold;">
                        Эффект победы
                    </label>
                    <textarea id="edit-event-victory-effect" class="enchanted-input event-description" 
                              placeholder="Эффект победы..." rows="2">${escapeHtml(event.victory_effect)}</textarea>
                </div>

                <div class="form-group">
                    <label style="display: block; margin-bottom: 8px; color: #5D4037; font-weight: bold;">
                        Эффект поражения
                    </label>
                    <textarea id="edit-event-defeat-effect" class="enchanted-input event-description" 
                              placeholder="Эффект поражения..." rows="2">${escapeHtml(event.defeat_effect)}</textarea>
                </div>

                <div class="form-group">
                    <label style="display: block; margin-bottom: 8px; color: #5D4037; font-weight: bold;">
                        Зависимости
                    </label>
                    <textarea id="edit-event-requirement" class="enchanted-input event-description" 
                              placeholder="Зависимости для события..." rows="2">${escapeHtml(event.requirement)}</textarea>
                </div>

                <div class="form-group" style="margin: 15px 0;">
                    <label class="file-label" style="display: flex; align-items: center; gap: 10px; cursor: pointer;">
                        <input type="checkbox" id="edit-event-init" ${event.init ? 'checked' : ''}>
                        Начальная карта
                    </label>
                </div>

                <div class="form-group">
                    <label class="file-label" for="edit-event-image" style="display: block; margin-bottom: 10px;">
                        📜 Изменить изображение (оставьте пустым, чтобы не менять)
                    </label>
                    <input type="file" id="edit-event-image" class="file-input" accept="image/*">
                    <div id="edit-file-name" style="margin-top: 5px; color: #5D4037; font-size: 14px;"></div>
                    
                    ${event.image_path ? `
                        <div style="margin-top: 10px;">
                            <div style="font-size: 14px; color: #5D4037; margin-bottom: 5px;">Текущее изображение:</div>
                            <img src="/static/events/${event.image_path}" 
                                 alt="${event.title}" 
                                 style="max-width: 200px; max-height: 150px; border-radius: 8px; border: 2px solid #8B4513;">
                        </div>
                    ` : ''}
                </div>
            </div>

            <div style="display: flex; gap: 15px; justify-content: center; margin-top: 20px;">
                <button id="edit-cancel-btn" class="magic-btn" 
                        style="background: linear-gradient(135deg, #8B4513, #a0522d);">
                    Отмена
                </button>
                <button id="edit-save-btn" class="magic-btn" 
                        style="background: linear-gradient(135deg, #1976d2, #1565c0);">
                    Сохранить
                </button>
            </div>
        </div>
    `;

    document.body.appendChild(modal);

    // Обработчик выбора файла
    const fileInput = document.getElementById('edit-event-image');
    const fileNameDisplay = document.getElementById('edit-file-name');

    fileInput.addEventListener('change', () => {
        if (fileInput.files.length > 0) {
            fileNameDisplay.textContent = `Выбран файл: ${fileInput.files[0].name}`;
        } else {
            fileNameDisplay.textContent = '';
        }
    });

    // Обработчики кнопок
    const cancelBtn = document.getElementById('edit-cancel-btn');
    const saveBtn = document.getElementById('edit-save-btn');

    cancelBtn.addEventListener('click', closeEditModal);
    saveBtn.addEventListener('click', () => saveEventChanges(event.id, event));

    modal.addEventListener('click', function (e) {
        if (e.target === modal) {
            closeEditModal();
        }
    });

    document.addEventListener('keydown', function escapeHandler(e) {
        if (e.key === 'Escape') {
            closeEditModal();
            document.removeEventListener('keydown', escapeHandler);
        }
    });

    // Фокус на первом поле
    setTimeout(() => {
        const firstInput = document.getElementById('edit-event-title');
        if (firstInput) firstInput.focus();
    }, 100);
}

async function saveEventChanges(eventId, currentEvent) {
    const title = document.getElementById('edit-event-title').value.trim();
    const description = document.getElementById('edit-event-description').value.trim();
    const type = document.getElementById('edit-event-type').value.trim();
    const difficult = document.getElementById('edit-event-difficulty').value.trim();
    const victory_effect = document.getElementById('edit-event-victory-effect').value.trim();
    const defeat_effect = document.getElementById('edit-event-defeat-effect').value.trim();
    const requirement = document.getElementById('edit-event-requirement').value.trim();
    const init = document.getElementById('edit-event-init').checked;

    const imageFile = document.getElementById('edit-event-image').files[0];

    // Валидация
    if (!title) {
        showNotification('Введите название события', 'error');
        return;
    }

    if (!description) {
        showNotification('Введите описание события', 'error');
        return;
    }

    if (!type) {
        showNotification('Введите тип события', 'error');
        return;
    }

    if (!difficult) {
        showNotification('Введите сложность события', 'error');
        return;
    }

    if (!victory_effect) {
        showNotification('Введите эффект победы', 'error');
        return;
    }

    if (!defeat_effect) {
        showNotification('Введите эффект поражения', 'error');
        return;
    }

    if (!requirement) {
        showNotification('Введите зависимости для события', 'error');
        return;
    }

    try {
        // Подготавливаем данные для обновления
        const updateData = {
            title,
            description,
            type,
            difficult,
            victory_effect,
            defeat_effect,
            requirement,
            init
        };

        // Если НЕ выбрано новое изображение, сохраняем текущий image_path
        if (!imageFile && currentEvent && currentEvent.image_path) {
            updateData.image_path = currentEvent.image_path;
        }

        // Обновляем данные события
        const updateRes = await fetch(`/api/admin/events/${eventId}`, {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify(updateData)
        });

        if (!updateRes.ok) {
            const errorText = await updateRes.text();
            throw new Error(errorText || 'Ошибка обновления события');
        }

        // Обновляем изображение, если выбрано новое
        if (imageFile) {
            await uploadEventImage(eventId, imageFile);
        }

        closeEditModal();
        await fetchEvents();
        showNotification('Событие успешно обновлено! ✨', 'success');

    } catch (err) {
        console.error('Ошибка обновления события:', err);
        showNotification('Ошибка обновления события: ' + err.message, 'error');
    }
}

function closeEditModal() {
    const modal = document.getElementById('edit-event-modal');
    if (modal) {
        modal.style.opacity = '0';
        modal.style.transition = 'opacity 0.3s ease';
        setTimeout(() => {
            modal.remove();
        }, 300);
    }
}

// Вспомогательная функция для экранирования HTML
function escapeHtml(unsafe) {
    if (unsafe === null || unsafe === undefined) return '';
    return unsafe
        .toString()
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/"/g, "&quot;")
        .replace(/'/g, "&#039;");
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
    const type = document.getElementById('new-event-type').value.trim();
    const difficult = document.getElementById('new-event-difficulty').value.trim();
    const victory_effect = document.getElementById('new-event-victory-effect').value.trim();
    const defeat_effect = document.getElementById('new-event-defeat-effect').value.trim();
    const requirement = document.getElementById('new-event-requirement').value.trim();
    const init = document.getElementById('new-event-init').checked;

    const imageFile = document.getElementById('new-event-image').files[0];

    if (!title) {
        showNotification('Введите название события', 'error');
        return;
    }

    if (!description) {
        showNotification('Введите описание события', 'error');
        return;
    }

    if (!type) {
        showNotification('Введите тип события', 'error');
        return;
    }

    if (!difficult) {
        showNotification('Введите сложность события', 'error');
        return;
    }

    if (!victory_effect) {
        showNotification('Введите эффект победы', 'error');
        return;
    }

    if (!defeat_effect) {
        showNotification('Введите эффект поражения', 'error');
        return;
    }

    if (!requirement) {
        showNotification('Введите зависимости для события', 'error');
        return;
    }

    try {
        // const payload = {
        //
        // }
        //
        console.log(JSON.stringify({
            title, description, type, difficult, victory_effect, defeat_effect, requirement, init
        }))
        // Создаём событие
        const eventRes = await fetch('/api/admin/events', {
            method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({
                title, description, type, difficult, victory_effect, defeat_effect, requirement, init
            })
        });
        if (!eventRes.ok) throw new Error(await eventRes.text());
        const eventData = await eventRes.json();

        // Загружаем картинку
        if (imageFile) {
            await uploadEventImage(eventData.id, imageFile);
        }

        clearEventForm();
        await fetchEvents();
        showNotification('Событие успешно создано! ✨', 'success');

    } catch (err) {
        console.error(err);
        showNotification('Ошибка создания события: ' + err.message, 'error');
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
                               cursor: pointer; font-weight: bold;">
                    Отмена
                </button>
                <button id="magic-reset-confirm-btn" 
                        style="padding: 12px 25px; background: linear-gradient(135deg, #c41e3a, #8B0000); 
                               color: white; border: 2px solid #ff6b6b; border-radius: 8px; 
                               cursor: pointer; font-weight: bold;">
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
            showNotification('Мир успешно перерождён!', 'success');
            await fetchTeams();
            await fetchEvents()
        } else {
            throw new Error('Ошибка сервера');
        }
    } catch (error) {
        console.error('Ошибка сброса команд:', error);
        closeResetModal();
        showNotification('Ошибка перерождения мира! 💥', 'error');
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
            showNotification('Событие успешно уничтожено! ✨', 'success');
            await fetchEvents();
        } else {
            throw new Error('Ошибка сервера');
        }
    } catch (error) {
        console.error('Ошибка удаления события:', error);
        closeModal();
        showNotification('Ошибка удаления события! 💥', 'error');
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
                               cursor: pointer; font-weight: bold;">
                    Отмена
                </button>
                <button id="magic-confirm-btn" 
                        style="padding: 12px 25px; background: linear-gradient(135deg, #c41e3a, #8B0000); 
                               color: white; border: 2px solid #ff6b6b; border-radius: 8px; 
                               cursor: pointer; font-weight: bold;">
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
function showNotification(text, type) {
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
        background: ${type === 'success' 
            ? 'linear-gradient(135deg, #4caf50, #388e3c)' 
            : 'linear-gradient(135deg, #920407, #ff6b6b)'
        };
        color: white;
        border-radius: 8px;
        box-shadow: 0 5px 15px rgba(0,0,0,0.3);
        z-index: 1001;
        border: 2px solid #ffd700;
        font-weight: bold;
        transform: translateX(100%);
        transition: transform 0.3s ease;
    `;


    message.innerHTML = `
        <span style="margin-right: 10px; font-size: 18px; color: #fff">${type === 'success' ? '✨' : '🔥'}</span>
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

function clearEventForm() {
    document.getElementById('new-event-title').value = '';
    document.getElementById('new-event-description').value = '';
    document.getElementById('new-event-type').value = '';
    document.getElementById('new-event-difficulty').value = '';
    document.getElementById('new-event-victory-effect').value = '';
    document.getElementById('new-event-defeat-effect').value = '';
    document.getElementById('new-event-requirement').value = '';
    document.getElementById('new-event-init').checked = false;
    document.getElementById('new-event-image').value = '';

    const preview = document.getElementById('image-preview');
    if (preview) {
        preview.innerHTML = '';
        preview.style.display = 'none';
    }
}