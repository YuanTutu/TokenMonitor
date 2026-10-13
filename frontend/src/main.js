import './style.css';
import {
    GetVersion, LoadConfig, SaveConfig, SaveProviderKey,
    RefreshAll, GetConfigPath, OpenConfigExternal, NotifyAlerts
} from '../wailsjs/go/main/App';

// ── 全局状态 ──────────────────────────────────────────

const state = {
    view: 'dashboard',      // 'dashboard' | 'config'
    version: '',
    config: null,           // ConfigView
    refreshResponse: null,  // RefreshResponse
    loading: false,
    refreshTimer: null,
    editingKey: null,       // 正在编辑的 provider key: 'glm'/'ark'/'ark_secret'/'deepseek'
};

// ── 工具函数 ──────────────────────────────────────────

// 根据百分比返回颜色档位
function pctColor(pct) {
    if (pct >= 80) return 'red';
    if (pct >= 50) return 'yellow';
    return 'green';
}

// isTerminal 当前是否为终端美学主题
function isTerminal() {
    return state.config && state.config.theme === 'terminal';
}

// setTheme 设置界面主题（'glass' 液态玻璃 / 'terminal' 终端美学）并持久化
window.setTheme = async function(theme) {
    const c = state.config;
    if (c.theme === theme) return;
    c.theme = theme;
    await SaveConfig(c);
    state.config = await LoadConfig();
    renderConfig(true); // 切主题后重播入场动画，切换感更明显
    showToast(theme === 'terminal' ? '已切换到终端界面' : '已切换到玻璃界面', 'success');
};

// renderBarChars 生成终端风格的字符进度条（█ 已用 / ░ 未用）
function renderBarChars(pct, width = 20) {
    const filled = Math.max(0, Math.min(width, Math.round(pct / 100 * width)));
    return '█'.repeat(filled) + '░'.repeat(width - filled);
}

// providerKeyByName 把 provider 展示名映射回配置 key（用于按 providerOrder 排序）
function providerKeyByName(name) {
    const map = {
        '智谱 GLM': 'glm',
        '火山方舟': 'ark',
        'DeepSeek': 'deepseek',
        'Kimi': 'kimi',
        'StepFun': 'stepfun',
        'MiniMax': 'minimax',
    };
    return map[name] || name;
}

// toast 提示
function showToast(msg, type = '') {
    const el = document.createElement('div');
    el.className = `toast ${type}`;
    el.textContent = msg;
    document.body.appendChild(el);
    setTimeout(() => el.remove(), 2500);
}

// bindCardTilt 给所有卡片绑定 hover 3D 倾斜效果。
// 鼠标在卡片内移动时，按相对位置计算 rotateX/rotateY（最大 ±4°），
// 通过 CSS 变量 --tilt-x/--tilt-y 驱动 .card-inner-3d 的 transform。
const MAX_TILT = 4;

function bindCardTilt() {
    document.querySelectorAll('.card').forEach(card => {
        // 避免重复绑定（DOM 重建后是新元素，无需解绑）
        card.addEventListener('mousemove', (e) => {
            const rect = card.getBoundingClientRect();
            const x = (e.clientX - rect.left) / rect.width - 0.5;   // -0.5 ~ 0.5
            const y = (e.clientY - rect.top) / rect.height - 0.5;
            card.style.setProperty('--tilt-x', (x * MAX_TILT).toFixed(2) + 'deg');
            card.style.setProperty('--tilt-y', (-y * MAX_TILT).toFixed(2) + 'deg');
        });
        card.addEventListener('mouseleave', () => {
            card.style.setProperty('--tilt-x', '0deg');
            card.style.setProperty('--tilt-y', '0deg');
        });
    });
}

// ── 主面板渲染 ────────────────────────────────────────

function renderDashboard(animate = false) {
    const r = state.refreshResponse;
    const app = document.getElementById('app');

    // 保存滚动位置：innerHTML 重建会丢失 .content 的 scrollTop。
    const prevContent = document.querySelector('.content');
    const savedScrollTop = prevContent ? prevContent.scrollTop : 0;

    let cardsHtml = '';
    let notifHtml = '';

    if (state.loading && !r) {
        cardsHtml = `<div class="state-msg"><span class="spinner"></span>正在查询套餐用量...</div>`;
    } else if (r && r.error) {
        cardsHtml = `<div class="state-msg">${r.error}</div>`;
    } else if (r && r.results && r.results.length > 0) {
        // 按配置的 providerOrder 排列卡片（纯展示排序，调整顺序后即时生效无需刷新）
        const order = state.config?.providerOrder || [];
        const sorted = [...r.results].sort((a, b) => {
            const ai = order.indexOf(providerKeyByName(a.providerName));
            const bi = order.indexOf(providerKeyByName(b.providerName));
            return (ai < 0 ? 999 : ai) - (bi < 0 ? 999 : bi);
        });
        cardsHtml = sorted.map(renderCard).join('');
    } else if (!state.loading) {
        cardsHtml = `<div class="state-msg">暂无数据。请按右上角设置按钮配置 Provider。</div>`;
    }

    // 通知区
    if (r && r.notifications && r.notifications.length > 0) {
        notifHtml = `<div class="notifications">` +
            r.notifications.map(n => {
                const cls = n.includes('⚠️') ? 'alert' : 'reset';
                return `<div class="notification ${cls}">${n}</div>`;
            }).join('') +
            `</div>`;
    }

    // animate=true（视图切换）时容器带 .anim-in 触发入场动画；
    // 数据刷新/交互更新不播动画，避免整页"刷新感"。
    app.className = isTerminal() ? 'theme-terminal' : '';
    app.innerHTML = `
        <div class="topbar">
            <div class="topbar-left">
                <span class="topbar-title">📊 AI 套餐余额监控</span>
                <span class="topbar-version">${state.version}</span>
            </div>
            <div class="topbar-right">
                <span class="topbar-status">${
                    r && r.updatedAt ? `最后刷新 ${r.updatedAt}` : ''
                } · ⏱${state.config ? state.config.refreshIntervalMinutes : 10}min</span>
                <div class="topbar-actions">
                    <button class="btn btn-icon" onclick="window.toggleNotifyAlert()" title="告警弹窗开关">${
                        state.config && state.config.notifyOnAlert ? '🔔' : '🔕'
                    }</button>
                    <button class="btn btn-primary" onclick="window.refresh()" ${
                        state.loading ? 'disabled' : ''
                    }>刷新</button>
                    <button class="btn btn-icon" onclick="window.showConfig()" title="配置">⚙️</button>
                </div>
            </div>
        </div>
        <div class="content${animate ? ' anim-in' : ''}">
            <div class="content-inner">
                ${notifHtml}
                ${cardsHtml}
            </div>
        </div>
    `;

    // 恢复滚动位置
    const newContent = document.querySelector('.content');
    if (newContent) {
        newContent.scrollTop = savedScrollTop;
    }
    // 绑定卡片 hover 3D 倾斜
    bindCardTilt();
}

// 渲染单个 provider 卡片。
function renderCard(result) {
    const title = result.providerName;
    const plan = result.plan ? `<span class="card-plan">${result.plan}</span>` : '';

    if (result.error) {
        return `
            <div class="card">
                <div class="card-inner-3d">
                    <div class="card-header">
                        <div><span class="card-title">${title}</span>${plan}</div>
                    </div>
                    <div class="card-error">❌ ${result.error}</div>
                </div>
            </div>
        `;
    }

    const itemsHtml = (result.items || []).map(renderItem).join('');

    return `
        <div class="card">
            <div class="card-inner-3d">
                <div class="card-header">
                    <div><span class="card-title">${title}</span>${plan}</div>
                    <span class="card-updated">${result.updatedAt ? new Date(result.updatedAt).toLocaleTimeString('zh-CN') : ''}</span>
                </div>
                ${itemsHtml}
            </div>
        </div>
    `;
}

// 渲染单个用量条目
function renderItem(item) {
    // text 类型：纯文本行
    if (item.format === 'text') {
        return `
            <div class="usage-text-row">
                <span class="usage-label">${item.label}</span>
                <span class="usage-text-value">${item.unit}</span>
            </div>
        `;
    }

    // count 类型无 limit（DeepSeek 余额）：文字行，低余额标红
    if (item.format === 'count' && item.limit === 0) {
        const lowBalance = item.used < (state.config?.alertBalanceThreshold || 10);
        const valColor = lowBalance ? 'red' : '';
        return `
            <div class="usage-text-row">
                <span class="usage-label">${item.label}</span>
                <span class="usage-text-value" style="${lowBalance ? 'color:var(--red)' : ''}">
                    ${item.used.toFixed(2)} ${item.unit}
                    ${item.resetsIn ? `<span class="usage-reset">· ${item.resetsIn}</span>` : ''}
                </span>
            </div>
        `;
    }

    // percent / count(有limit)：带进度条
    const pct = item.percent;
    const color = pctColor(pct);
    const valueStr = item.format === 'count'
        ? `${item.used.toFixed(0)}/${item.limit.toFixed(0)} ${item.unit}`
        : '';

    // 终端主题：字符进度条（█░）；玻璃主题：圆角渐变进度条
    const barHtml = isTerminal()
        ? `<span class="bar-chars ${color}">${renderBarChars(pct, 20)}</span>`
        : `<div class="usage-bar">
               <div class="usage-bar-fill ${color}" style="width: ${Math.min(pct, 100)}%"></div>
           </div>`;

    return `
        <div class="usage-item">
            <span class="usage-label">${item.label}</span>
            <div class="usage-bar-wrap">
                ${barHtml}
            </div>
            <div class="usage-value">
                <span class="usage-percent ${color}">${pct.toFixed(1)}%</span>
                ${item.resetsIn ? `<span class="usage-reset">${item.resetsIn}</span>` : ''}
                ${valueStr ? `<span class="usage-reset">${valueStr}</span>` : ''}
            </div>
        </div>
    `;
}

// ── 配置页渲染 ────────────────────────────────────────

function renderConfig(animate = false) {
    const c = state.config;
    if (!c) {
        document.getElementById('app').innerHTML = `<div class="state-msg">配置加载失败</div>`;
        return;
    }

    // 保存滚动位置：innerHTML 重建会丢失 .content 的 scrollTop，
    // 导致每次点 +/- 调整时 UI 抖动/下沉。渲染后恢复。
    const prevContent = document.querySelector('.content');
    const savedScrollTop = prevContent ? prevContent.scrollTop : 0;

    // provider 定义表（key → 展示信息），按 c.providerOrder 排列展示
    const providerDefs = {
        glm:      { key: 'glm', name: '智谱 GLM', credField: 'glm' },
        ark:      { key: 'ark', name: '火山方舟', credField: 'ark', hasSecret: true },
        deepseek: { key: 'deepseek', name: 'DeepSeek', credField: 'deepseek' },
        kimi:     { key: 'kimi', name: 'Kimi', credField: 'kimi' },
        stepfun:  { key: 'stepfun', name: 'StepFun', credField: 'stepfun' },
        minimax:  { key: 'minimax', name: 'MiniMax', credField: 'minimax' },
    };
    const order = c.providerOrder && c.providerOrder.length ? c.providerOrder : Object.keys(providerDefs);
    const providers = order
        .filter(k => providerDefs[k])
        .map(k => ({ ...providerDefs[k], ...c.providers[k] }));

    const providerRows = providers.map((p, idx) => {
        const statusBadge = !p.hasKey
            ? '<span class="badge badge-warn">未配置</span>'
            : (p.enabled
                ? '<span class="badge badge-ok">已启用</span>'
                : '<span class="badge badge-off">已停用</span>');

        const keyDisplay = p.hasKey
            ? `<span class="config-key-mask">${p.keyMask}</span>`
            : '<span class="config-key-mask" style="color:var(--red)">未设置</span>';

        let editButtons = `
            <button class="btn btn-sm" onclick="window.editKey('${p.credField}')">编辑 Key</button>
        `;
        if (p.hasSecret) {
            editButtons += `
                <button class="btn btn-sm" onclick="window.editKey('ark_secret')">编辑 Secret</button>
            `;
        }

        // 排序按钮：首行禁用上移，末行禁用下移
        const sortButtons = `
            <button class="btn btn-sm btn-icon" onclick="window.moveProvider('${p.key}', -1)" ${idx === 0 ? 'disabled' : ''} title="上移">↑</button>
            <button class="btn btn-sm btn-icon" onclick="window.moveProvider('${p.key}', 1)" ${idx === providers.length - 1 ? 'disabled' : ''} title="下移">↓</button>
        `;

        return `
            <div class="config-row">
                <div class="config-row-left">
                    ${sortButtons}
                    <div class="toggle ${p.enabled ? 'on' : ''}" onclick="window.toggleProvider('${p.key}')"></div>
                    <span class="config-row-label">${p.name}</span>
                    ${statusBadge}
                </div>
                <div class="config-row-right">
                    ${keyDisplay}
                    ${editButtons}
                </div>
            </div>
        `;
    }).join('');

    document.getElementById('app').className = isTerminal() ? 'theme-terminal' : '';
    document.getElementById('app').innerHTML = `
        <div class="topbar">
            <div class="topbar-left">
                <span class="topbar-title">⚙️ 配置管理</span>
                <span class="topbar-version">${state.version}</span>
            </div>
            <div class="topbar-right">
                <div class="topbar-actions">
                    <button class="btn" onclick="window.showDashboard()">← 返回</button>
                    <button class="btn btn-icon" onclick="window.openExternal()" title="外部编辑器">📝</button>
                </div>
            </div>
        </div>
        <div class="content${animate ? ' anim-in' : ''}">
            <div class="content-inner">
                <div class="config-section">
                    <div class="config-section-title">Provider 凭据</div>
                    <div class="config-section-hint">↑↓ 调整卡片顺序，主面板立即生效</div>
                    ${providerRows}
                </div>

                <div class="config-section">
                    <div class="config-section-title">刷新与提醒</div>
                    <div class="config-row">
                        <div class="config-row-left">
                            <span class="config-row-label">自动刷新间隔</span>
                        </div>
                        <div class="config-row-right threshold-control">
                            <button class="btn btn-sm" onclick="window.adjustInterval(-1)">−</button>
                            <span class="threshold-value">${c.refreshIntervalMinutes} 分钟</span>
                            <button class="btn btn-sm" onclick="window.adjustInterval(1)">+</button>
                        </div>
                    </div>
                    <div class="config-row">
                        <div class="config-row-left">
                            <span class="config-row-label">重置提醒</span>
                        </div>
                        <div class="config-row-right">
                            <div class="toggle ${c.notifyOnReset ? 'on' : ''}" onclick="window.toggleNotifyReset()"></div>
                        </div>
                    </div>
                    <div class="config-row">
                        <div class="config-row-left">
                            <span class="config-row-label">界面主题</span>
                        </div>
                        <div class="config-row-right">
                            <button class="btn btn-sm ${c.theme !== 'terminal' ? 'btn-primary' : ''}" onclick="window.setTheme('glass')">液态玻璃</button>
                            <button class="btn btn-sm ${c.theme === 'terminal' ? 'btn-primary' : ''}" onclick="window.setTheme('terminal')">终端</button>
                        </div>
                    </div>
                </div>

                <div class="config-section">
                    <div class="config-section-title">告警阈值</div>
                    <div class="config-row">
                        <div class="config-row-left">
                            <span class="config-row-label">用量告警阈值</span>
                        </div>
                        <div class="config-row-right threshold-control">
                            <button class="btn btn-sm" onclick="window.adjustPctThreshold(-5)">−</button>
                            <span class="threshold-value">${c.alertPercentThreshold.toFixed(0)}%</span>
                            <button class="btn btn-sm" onclick="window.adjustPctThreshold(5)">+</button>
                        </div>
                    </div>
                    <div class="config-row">
                        <div class="config-row-left">
                            <span class="config-row-label">低余额告警阈值</span>
                        </div>
                        <div class="config-row-right threshold-control">
                            <button class="btn btn-sm" onclick="window.adjustBalThreshold(-5)">−</button>
                            <span class="threshold-value">${c.alertBalanceThreshold.toFixed(0)} CNY</span>
                            <button class="btn btn-sm" onclick="window.adjustBalThreshold(5)">+</button>
                        </div>
                    </div>
                </div>

                <div id="modal-container"></div>
            </div>
        </div>
    `;

    // 恢复滚动位置（避免 innerHTML 重建后 UI 抖动/下沉）
    const newContent = document.querySelector('.content');
    if (newContent) {
        newContent.scrollTop = savedScrollTop;
    }
}

// ── 编辑 Key 模态框 ───────────────────────────────────

function showEditKeyModal(credField) {
    state.editingKey = credField;
    const labels = {
        'glm': '智谱 GLM API Key',
        'ark': '火山方舟 AccessKey ID',
        'ark_secret': '火山方舟 Secret Access Key',
        'deepseek': 'DeepSeek API Key',
        'kimi': 'Kimi (Moonshot) API Key',
        'stepfun': 'StepFun API Key',
        'minimax': 'MiniMax Token Plan 订阅 Key',
    };
    const title = labels[credField] || '凭据';

    const container = document.getElementById('modal-container');
    container.innerHTML = `
        <div class="modal-overlay" onclick="window.cancelEditKey(event)">
            <div class="modal" onclick="event.stopPropagation()">
                <div class="modal-title">${title}</div>
                <div class="modal-body">
                    <input type="text" class="modal-input" id="key-input"
                           placeholder="粘贴 ${title}" autocomplete="off" />
                    <div class="modal-hint">输入新值后回车保存，留空则清除</div>
                </div>
                <div class="modal-actions">
                    <button class="btn" onclick="window.cancelEditKey()">取消</button>
                    <button class="btn btn-primary" onclick="window.saveKey()">保存</button>
                </div>
            </div>
        </div>
    `;
    document.getElementById('key-input').focus();
}

// ── 全局动作（挂到 window 供 onclick 调用）────────────

window.refresh = async function() {
    if (state.loading) return;
    state.loading = true;
    if (state.view === 'dashboard') renderDashboard();
    const prevNotifs = state.refreshResponse ? (state.refreshResponse.notifications || []) : [];
    try {
        state.refreshResponse = await RefreshAll();
    } catch (e) {
        state.refreshResponse = { error: String(e), results: [], notifications: [] };
    }
    state.loading = false;
    if (state.view === 'dashboard') renderDashboard();

    // 系统通知：仅弹"本次新增"的通知，避免每次刷新重复弹窗
    const curNotifs = state.refreshResponse.notifications || [];
    const newNotifs = curNotifs.filter(n => !prevNotifs.includes(n));
    if (newNotifs.length > 0) {
        NotifyAlerts(newNotifs).catch(() => {});  // 通知失败不影响主流程
    }

    scheduleRefresh();
};

window.showConfig = async function() {
    state.view = 'config';
    state.config = await LoadConfig();
    renderConfig(true); // 视图切换：播入场动画
};

window.showDashboard = function() {
    state.view = 'dashboard';
    renderDashboard(true); // 视图切换：播入场动画
};

window.toggleProvider = async function(key) {
    const c = state.config;
    c.providers[key].enabled = !c.providers[key].enabled;
    await SaveConfig(c);
    state.config = await LoadConfig();
    renderConfig();
    showToast(`${c.providers[key].enabled ? '已启用' : '已停用'}`, 'success');
};

// moveProvider 上移/下移 provider 展示顺序（delta: -1 上移, +1 下移）
window.moveProvider = async function(key, delta) {
    const c = state.config;
    const order = [...(c.providerOrder || [])];
    const idx = order.indexOf(key);
    const target = idx + delta;
    if (idx < 0 || target < 0 || target >= order.length) return;
    [order[idx], order[target]] = [order[target], order[idx]];
    c.providerOrder = order;
    await SaveConfig(c);
    state.config = await LoadConfig();
    renderConfig();
};

window.editKey = function(credField) {
    showEditKeyModal(credField);
};

window.cancelEditKey = function(e) {
    if (e && e.target.classList && !e.target.classList.contains('modal-overlay')) return;
    state.editingKey = null;
    document.getElementById('modal-container').innerHTML = '';
};

window.saveKey = async function() {
    const val = document.getElementById('key-input').value.trim();
    try {
        await SaveProviderKey(state.editingKey, val);
        state.config = await LoadConfig();
        document.getElementById('modal-container').innerHTML = '';
        state.editingKey = null;
        renderConfig();
        showToast('凭据已保存', 'success');
        // 保存后自动刷新一次
        window.refresh();
    } catch (e) {
        showToast('保存失败: ' + String(e), 'error');
    }
};

window.adjustInterval = async function(delta) {
    const c = state.config;
    c.refreshIntervalMinutes = Math.max(1, c.refreshIntervalMinutes + delta);
    await SaveConfig(c);
    state.config = await LoadConfig();
    renderConfig();
};

window.toggleNotifyReset = async function() {
    const c = state.config;
    c.notifyOnReset = !c.notifyOnReset;
    await SaveConfig(c);
    state.config = await LoadConfig();
    renderConfig();
};

window.toggleNotifyAlert = async function() {
    const c = state.config;
    c.notifyOnAlert = !c.notifyOnAlert;
    await SaveConfig(c);
    state.config = await LoadConfig();
    // 开关位于首页顶栏，切换后重渲染当前视图（不播动画，只换图标）
    if (state.view === 'config') {
        renderConfig();
    } else {
        renderDashboard();
    }
    showToast(c.notifyOnAlert ? '告警弹窗已开启' : '告警弹窗已关闭', 'success');
};

window.adjustPctThreshold = async function(delta) {
    const c = state.config;
    c.alertPercentThreshold = Math.min(100, Math.max(1, c.alertPercentThreshold + delta));
    await SaveConfig(c);
    state.config = await LoadConfig();
    renderConfig();
};

window.adjustBalThreshold = async function(delta) {
    const c = state.config;
    c.alertBalanceThreshold = Math.max(0, c.alertBalanceThreshold + delta);
    await SaveConfig(c);
    state.config = await LoadConfig();
    renderConfig();
};

window.openExternal = async function() {
    try {
        await OpenConfigExternal();
        showToast('编辑器已打开，修改后按刷新', 'success');
    } catch (e) {
        showToast('打开失败: ' + String(e), 'error');
    }
};

// 回车保存 key
document.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && state.editingKey && document.activeElement.id === 'key-input') {
        window.saveKey();
    }
    if (e.key === 'Escape' && state.editingKey) {
        window.cancelEditKey();
    }
});

// ── 自动刷新调度 ──────────────────────────────────────

function scheduleRefresh() {
    if (state.refreshTimer) clearTimeout(state.refreshTimer);
    const minutes = state.config ? state.config.refreshIntervalMinutes : 10;
    state.refreshTimer = setTimeout(() => {
        window.refresh();
    }, minutes * 60 * 1000);
}

// ── 初始化 ────────────────────────────────────────────

// ensureBgGlow 一次性创建背景光斑层（挂在 body 下、#app 之外）。
// 光斑动画永续播放，不受视图渲染重建 DOM 影响，避免"刷新闪动"。
function ensureBgGlow() {
    if (document.querySelector('.bg-glow')) return;
    ['g1', 'g2', 'g3'].forEach(cls => {
        const el = document.createElement('div');
        el.className = `bg-glow ${cls}`;
        document.body.appendChild(el);
    });
}

async function init() {
    ensureBgGlow();
    state.version = await GetVersion().catch(() => 'dev');
    state.config = await LoadConfig().catch(() => null);
    renderDashboard(true); // 首次进入播入场动画
    // 首次自动刷新
    window.refresh();
}

init();
