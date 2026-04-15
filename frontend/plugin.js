// tasks-plugin/frontend/plugin.js
// Plugin-specific JS for the tasks kanban board.
// Depends on MC namespace from main.js (loaded first).

function groupByStatus(items) {
  const map = {};
  const order = [];
  for (const item of items) {
    const status = item.status || 'other';
    if (!map[status]) { map[status] = []; order.push(status); }
    map[status].push(item);
  }
  return order.map(s => ({ id: s, title: s.replace(/_/g, ' '), items: map[s] }));
}

// Format ISO date as "Mar 23, 14:30"
function formatDateTime(isoStr) {
  if (!isoStr) return '—';
  const d = new Date(isoStr);
  if (isNaN(d)) return '—';
  const months = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
  return months[d.getMonth()] + ' ' + d.getDate() + ', ' + String(d.getHours()).padStart(2,'0') + ':' + String(d.getMinutes()).padStart(2,'0');
}

function isOutsideDialog(dialog, x, y) {
  const r = dialog.getBoundingClientRect();
  return x < r.left || x > r.right || y < r.top || y > r.bottom;
}

function bindBackdropClose(dialog, onBackdropClose) {
  dialog._mousedownOnBackdrop = false;
  dialog.addEventListener('mousedown', e => {
    dialog._mousedownOnBackdrop = isOutsideDialog(dialog, e.clientX, e.clientY);
  }, true);
  dialog.addEventListener('click', e => {
    const shouldClose = dialog._mousedownOnBackdrop && isOutsideDialog(dialog, e.clientX, e.clientY);
    dialog._mousedownOnBackdrop = false;
    if (shouldClose) onBackdropClose();
  });
  dialog.addEventListener('cancel', e => {
    e.preventDefault();
  });
}

function renderKanban(data, ctx) {
  let columns = [];
  if (data && data.columns) columns = data.columns;
  else if (Array.isArray(data)) columns = data;
  else if (data && typeof data === 'object') {
    const entries = Object.entries(data);
    const arrayEntry = entries.find(([, v]) => Array.isArray(v));
    if (arrayEntry) columns = groupByStatus(arrayEntry[1]);
    else return MC.renderers['table'] ? MC.renderers['table'](data, ctx) : '';
  }

  // Build task type lookup from board data.
  const taskTypeMap = {};
  if (data && data.task_types) {
    for (const tt of data.task_types) { taskTypeMap[String(tt.id)] = tt; }
  }
  // Store on ctx for detail view.
  ctx._taskTypeMap = taskTypeMap;
  ctx._columns = columns;

  const createTool = findTool(ctx.tools, 'create_', 'add_');
  const deleteTool = findTool(ctx.tools, 'delete_', 'remove_');

  let html = '<div class="page-toolbar">';
  if (createTool) {
    const schema = ctx.tools.find(t => t.name === createTool);
    html += `<button class="button button-primary" data-action="show-create">${MC.t('tasks.create', '+ New')}</button>`;
    html += renderCreateForm(createTool, schema, ctx);
  }
  html += '<div id="kanban-project-filters" style="display:flex;gap:0.375rem;flex-wrap:wrap;"></div>';
  html += `<button class="button button-ghost" data-action="open-board-settings" title="${MC.t('tasks.board_settings', 'Board Settings')}"><svg class="icon" width="16" height="16"><use href="/static/icons/sprite.svg#settings"></use></svg></button>`;
  html += '</div>';

  html += '<div class="kanban-board">' + columns.map(col => {
    const items = col.items || [];
    const colColor = col.color ? ` style="border-top:3px solid ${MC.esc(col.color)}"` : '';
    return `<div class="kanban-column" draggable="true" data-col-status="${MC.esc(col.id)}" data-col-db-id="${col.db_id || ''}" data-col-pos="${col.position != null ? col.position : ''}">
      <div class="kanban-column-header"${colColor}>
        <span class="kanban-column-title">${MC.esc(col.title || col.id || '—')}</span>
        <div class="kanban-column-header-right">
          <span class="badge badge-default">${items.length}</span>
          ${createTool ? `<button class="button button-ghost kanban-col-add-btn" data-action="show-create-in-col" data-col-status="${MC.esc(col.id)}" title="Add task">+</button>` : ''}
        </div>
      </div>
      <div class="kanban-column-body" data-drop-status="${MC.esc(col.id)}">
        ${items.length === 0 ? '<p class="text-muted-foreground text-sm kanban-empty-hint" style="padding:0.5rem;">Empty</p>' :
          items.map(item => {
            const title = item.title || item.name || item.id || '—';
            const idBadge = `<span class="task-id-badge">#${item.id}</span>`;
            const priority = item.priority != null ? `<span class="badge badge-default">P${item.priority}</span>` : '';
            const projectBadge = '';
            // Task type badge
            let typeBadge = '';
            if (item.type_id && taskTypeMap[String(item.type_id)]) {
              const tt = taskTypeMap[String(item.type_id)];
              const bgStyle = tt.color ? ` style="background:${MC.esc(tt.color)};color:#fff"` : '';
              typeBadge = `<span class="badge"${bgStyle}>${MC.esc(tt.label)}</span>`;
            }
            let deleteHtml = '';
            if (deleteTool) {
              deleteHtml = `<button class="button button-ghost button-xs" data-action="delete" data-tool="${MC.esc(deleteTool)}" data-id="${item.id}" data-name="${MC.esc(String(item.title || ''))}" title="${MC.t('common.delete', 'Delete')}">&times;</button>`;
            }
            const extBadges = renderExtensionBadges(ctx.extensions, 'task-card-badge', item.id);
            const extActions = renderExtensionActions(ctx.extensions, 'task-card-actions', item.id);
            return `<div class="kanban-card" draggable="true" data-action="open-detail" data-task-id="${item.id}" data-task-status="${MC.esc(col.id)}">
              <div class="kanban-card-title">${idBadge}<span class="font-medium">${MC.esc(String(title))}</span> ${priority}${typeBadge}${projectBadge}${extBadges}</div>
              ${(deleteHtml || extActions) ? `<div class="kanban-card-actions">${extActions}${deleteHtml}</div>` : ''}
            </div>`;
          }).join('')}
      </div>
    </div>`;
  }).join('');
  // Ghost "add column" placeholder
  html += `<div class="kanban-column kanban-column-ghost" data-action="open-board-settings-add-col" title="Add column">
    <div class="kanban-column-ghost-inner">
      <span class="kanban-ghost-plus">+</span>
    </div>
  </div>`;
  html += '</div>';
  return html;
}

function bindKanbanDragDrop(el, ctx) {
  const board = el.querySelector('.kanban-board');
  if (!board) return;

  const updateTool = findTool(ctx.tools, 'update_', 'edit_');
  let dragCard = null;
  let dragColumn = null;

  // --- Task card drag ---
  board.querySelectorAll('.kanban-card[draggable]').forEach(card => {
    card.addEventListener('dragstart', e => {
      dragCard = card;
      dragColumn = null;
      card.classList.add('dragging');
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', card.dataset.taskId);
      e.stopPropagation();
    });
    card.addEventListener('dragend', () => {
      card.classList.remove('dragging');
      dragCard = null;
      board.querySelectorAll('.kanban-column-body').forEach(b => b.classList.remove('drag-over'));
    });
  });

  // --- Column body as drop target for cards ---
  board.querySelectorAll('.kanban-column-body[data-drop-status]').forEach(body => {
    body.addEventListener('dragover', e => {
      if (!dragCard) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';
      body.classList.add('drag-over');
    });
    body.addEventListener('dragleave', e => {
      if (!dragCard) return;
      if (!body.contains(e.relatedTarget)) body.classList.remove('drag-over');
    });
    body.addEventListener('drop', e => {
      e.preventDefault();
      body.classList.remove('drag-over');
      if (!dragCard || !updateTool) return;
      const taskId = Number(dragCard.dataset.taskId);
      const oldStatus = dragCard.dataset.taskStatus;
      const newStatus = body.dataset.dropStatus;
      if (oldStatus === newStatus) return;
      const emptyHint = body.querySelector('.kanban-empty-hint');
      if (emptyHint) emptyHint.remove();
      body.appendChild(dragCard);
      dragCard.dataset.taskStatus = newStatus;
      updateColumnCounts(board);
      MC.callAction(ctx, updateTool, { id: taskId, status: newStatus })
        .then(() => MC.showToast('Task moved'))
        .catch(() => { MC.showToast('Failed to move task', 'error'); MC.reloadPage(); });
    });
  });

  // --- Column drag (reorder) ---
  board.querySelectorAll('.kanban-column[draggable]').forEach(col => {
    col.addEventListener('dragstart', e => {
      if (dragCard) return;
      dragColumn = col;
      col.classList.add('dragging-column');
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', 'column');
    });
    col.addEventListener('dragend', () => {
      col.classList.remove('dragging-column');
      dragColumn = null;
      board.querySelectorAll('.kanban-column').forEach(c => {
        c.classList.remove('col-drop-left', 'col-drop-right');
      });
    });
    col.addEventListener('dragover', e => {
      if (!dragColumn || dragColumn === col) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';
      const rect = col.getBoundingClientRect();
      const mid = rect.left + rect.width / 2;
      const isLeft = e.clientX < mid;
      col.classList.toggle('col-drop-left', isLeft);
      col.classList.toggle('col-drop-right', !isLeft);
    });
    col.addEventListener('dragleave', e => {
      if (!dragColumn) return;
      if (!col.contains(e.relatedTarget)) {
        col.classList.remove('col-drop-left', 'col-drop-right');
      }
    });
    col.addEventListener('drop', e => {
      e.preventDefault();
      const isLeft = col.classList.contains('col-drop-left');
      col.classList.remove('col-drop-left', 'col-drop-right');
      if (!dragColumn || dragColumn === col) return;

      // Snapshot original order so we can revert the DOM if the DB update fails.
      const originalOrder = [...board.querySelectorAll('.kanban-column[data-col-db-id]')];
      const originalPositions = new Map(
        originalOrder.map((c, i) => [Number(c.dataset.colDbId), i])
      );

      if (isLeft) {
        col.before(dragColumn);
      } else {
        col.after(dragColumn);
      }

      const reordered = [...board.querySelectorAll('.kanban-column[data-col-db-id]')];
      // Only send updates for columns whose position actually changed — matches
      // the settings-screen behavior and avoids N concurrent writes hammering
      // the plugin SQLite file for positions that didn't move.
      const updates = [];
      reordered.forEach((c, i) => {
        const dbId = Number(c.dataset.colDbId);
        if (!dbId) return;
        if (originalPositions.get(dbId) !== i) {
          updates.push(MC.callAction(ctx, 'update_board_column', { id: dbId, position: i }));
        }
      });
      if (updates.length === 0) return;

      Promise.all(updates)
        .then(() => {
          // Persist the new position on the DOM so subsequent reorders compare
          // against the freshly-saved baseline rather than the stale one.
          reordered.forEach((c, i) => { c.dataset.colPos = String(i); });
          MC.showToast('Columns reordered');
        })
        .catch(err => {
          // Revert the DOM to the pre-drop order.
          const parent = board;
          originalOrder.forEach(c => parent.appendChild(c));
          MC.showToast('Failed to reorder columns: ' + (err && err.message ? err.message : 'server error'), 'error');
        });
    });
  });
}

function updateColumnCounts(board) {
  board.querySelectorAll('.kanban-column').forEach(col => {
    const body = col.querySelector('.kanban-column-body');
    const badge = col.querySelector('.kanban-column-header .badge');
    if (body && badge) {
      const count = body.querySelectorAll('.kanban-card').length;
      badge.textContent = count;
    }
  });
}

function openTaskDetail(taskId, ctx) {
  MC.callAction(ctx, 'get_task_detail', { id: taskId }).then(data => {
    let dialog = document.getElementById('mc-task-detail');
    if (!dialog) {
      dialog = document.createElement('dialog');
      dialog.id = 'mc-task-detail';
      dialog.className = 'mc-dialog task-detail-dialog';
      document.body.appendChild(dialog);
      bindBackdropClose(dialog, () => {
        if (dialog._closeDetail) dialog._closeDetail();
      });
    }

    const task = data.task;
    const comments = data.comments || [];
    const typeInfo = data.type_info;
    const columns = data.columns || [];

    // Update URL with task deep link
    const deepUrl = new URL(window.location.href);
    deepUrl.searchParams.set('task', taskId);
    history.replaceState(null, '', deepUrl.toString());

    // Header
    let html = '<div class="task-detail-header">';
    html += `<span class="task-id-badge">#${task.id}</span>`;
    html += `<span class="editable-title font-medium" style="font-size:1.375rem;" title="Click to edit">${MC.esc(task.title)}</span>`;
    html += '</div>';
    html += `<button class="button button-ghost button-xs detail-copy-link-btn" id="detail-copy-link" title="Copy link">`;
    html += `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>`;
    html += ` Copy Link</button>`;

    // Meta badges
    html += '<div style="display:flex;gap:0.375rem;flex-wrap:wrap;margin-bottom:1rem;">';
    html += `<span class="badge badge-${MC.esc(task.status)}">${MC.esc(task.status.replace(/_/g, ' '))}</span>`;
    html += `<span class="badge badge-default">P${task.priority}</span>`;
    if (typeInfo) {
      const bgStyle = typeInfo.color ? ` style="background:${MC.esc(typeInfo.color)};color:#fff"` : '';
      html += `<span class="badge"${bgStyle}>${MC.esc(typeInfo.label)}</span>`;
    }
    if (task.due_date) html += `<span class="badge badge-default">${MC.esc(task.due_date)}</span>`;
    html += '</div>';

    // Status, Priority, Type change
    html += '<div style="margin-bottom:1rem;display:flex;align-items:center;gap:1rem;flex-wrap:wrap;">';
    if (columns.length > 0) {
      html += '<div style="display:flex;align-items:center;gap:0.5rem;">';
      html += '<span class="text-sm text-muted-foreground">Status:</span>';
      html += `<select id="detail-status-select" class="form-select" style="width:auto;">`;
      for (const col of columns) {
        const sel = col.name === task.status ? ' selected' : '';
        html += `<option value="${MC.esc(col.name)}"${sel}>${MC.esc(col.label)}</option>`;
      }
      html += '</select></div>';
    }
    // Priority
    html += '<div style="display:flex;align-items:center;gap:0.5rem;">';
    html += '<span class="text-sm text-muted-foreground">Priority:</span>';
    html += `<select id="detail-priority-select" class="form-select" style="width:auto;">`;
    for (let p = 1; p <= 5; p++) {
      const sel = p === task.priority ? ' selected' : '';
      html += `<option value="${p}"${sel}>P${p}</option>`;
    }
    html += '</select></div>';
    // Type
    const taskTypes = data.task_types || [];
    if (taskTypes.length > 0) {
      html += '<div style="display:flex;align-items:center;gap:0.5rem;">';
      html += '<span class="text-sm text-muted-foreground">Type:</span>';
      html += `<select id="detail-type-select" class="form-select" style="width:auto;">`;
      html += '<option value="">— none —</option>';
      for (const tt of taskTypes) {
        const sel = task.type_id && String(task.type_id) === String(tt.id) ? ' selected' : '';
        html += `<option value="${tt.id}"${sel}>${MC.esc(tt.label)}</option>`;
      }
      html += '</select></div>';
    }
    html += '</div>';

    // Description
    html += '<div class="task-detail-description-wrap">';
    if (task.description) {
      html += `<div class="task-detail-description markdown-body" id="desc-view">${MC.esc(task.description)}</div>`;
      html += `<button class="button button-ghost button-xs edit-desc-btn" title="${MC.t('tasks.edit_description', 'Edit description')}">${MC.t('common.edit', 'Edit')}</button>`;
    } else {
      html += `<button class="button button-ghost button-xs edit-desc-btn" title="Add description">+ Add description</button>`;
    }
    html += '</div>';

    // Tracked Time section (loaded async from pomodoro-plugin)
    html += '<div id="detail-tracked-time" style="margin:1rem 0;padding:0.75rem 1rem;border:1px solid hsl(var(--border));border-radius:var(--radius);font-size:0.8125rem;"></div>';

    // Comments
    html += '<h3 class="text-sm font-semibold" style="margin:1rem 0 0.5rem;">Comments</h3>';
    if (comments.length === 0) {
      html += `<p class="text-muted-foreground text-sm">${MC.t('tasks.no_comments', 'No comments yet.')}</p>`;
    } else {
      html += '<div class="comment-list">';
      for (const c of comments) {
        const authorBadge = c.author_type === 'agent' ? ' <span class="badge badge-default">agent</span>' : '';
        const isOwner = c.author_type !== 'agent' && (ctx.userId ? c.author_id === ctx.userId : c.author_name === (ctx.username || ''));
        const editBtn = isOwner ? `<button class="button button-ghost button-xs comment-edit-btn" data-comment-id="${c.id}" style="margin-left:auto;">${MC.t('common.edit', 'Edit')}</button>` : '';
        html += `<div class="comment-item" data-comment-id="${c.id}">`;
        html += `<div class="comment-meta"><strong>${MC.esc(c.author_name)}</strong>${authorBadge} · ${formatDateTime(c.created_at)}${editBtn}</div>`;
        html += `<div class="comment-body" id="comment-body-${c.id}">${MC.esc(c.body)}</div>`;
        html += `</div>`;
      }
      html += '</div>';
    }

    // Comment form
    html += '<div class="comment-form">';
    html += `<div id="detail-comment-editor" data-editor data-field-name="body" data-preview-url="/api/preview-markdown" data-csrf-token="${MC.esc(ctx.csrf || '')}"></div>`;
    html += '<div style="margin-top:0.5rem;display:flex;gap:0.5rem;align-items:center;">';
    html += '<button class="button button-primary button-sm" id="detail-comment-submit">Comment</button>';
    html += '</div></div>';

    // Close button
    html += `<div style="margin-top:1rem;text-align:right;"><button class="button button-ghost" id="detail-close">${MC.t('common.close', 'Close')}</button></div>`;

    dialog.innerHTML = html;
    dialog._mousedownOnBackdrop = false;

    // Render markdown in description and comment bodies
    if (typeof window.fetchMarkdownPreview === 'function' && ctx.csrf) {
      const descEl = dialog.querySelector('#desc-view');
      if (descEl && task.description) window.fetchMarkdownPreview(task.description, descEl, '/api/preview-markdown', ctx.csrf);
      comments.forEach(c => {
        const el = dialog.querySelector(`#comment-body-${c.id}`);
        if (el && c.body) window.fetchMarkdownPreview(c.body, el, '/api/preview-markdown', ctx.csrf);
      });
    }
    dialog.showModal();
    dialog.focus();

    // Bind status change
    const statusSelect = dialog.querySelector('#detail-status-select');
    if (statusSelect) {
      statusSelect.addEventListener('change', () => {
        const newStatus = statusSelect.value;
        if (newStatus !== task.status) {
          MC.callAction(ctx, 'update_task', { id: task.id, status: newStatus }).then(MC.reloadPage);
        }
      });
    }
    const prioSelect = dialog.querySelector('#detail-priority-select');
    if (prioSelect) {
      prioSelect.addEventListener('change', () => {
        const newPrio = Number(prioSelect.value);
        if (newPrio !== task.priority) {
          MC.callAction(ctx, 'update_task', { id: task.id, priority: newPrio })
            .then(() => { task.priority = newPrio; MC.showToast('Priority updated'); MC.reloadPage(); });
        }
      });
    }
    const typeSelect = dialog.querySelector('#detail-type-select');
    if (typeSelect) {
      typeSelect.addEventListener('change', () => {
        const val = typeSelect.value;
        const newTypeId = val ? Number(val) : null;
        MC.callAction(ctx, 'update_task', { id: task.id, type_id: newTypeId })
          .then(() => { MC.showToast('Type updated'); MC.reloadPage(); });
      });
    }

    // Bind title inline edit
    const titleHeader = dialog.querySelector('.task-detail-header');
    titleHeader.addEventListener('click', function(e) {
      if (!e.target.closest('.editable-title')) return;
      if (titleHeader.querySelector('.inline-edit-input')) return;
      const span = titleHeader.querySelector('.editable-title');
      const input = document.createElement('input');
      input.type = 'text';
      input.value = task.title;
      input.className = 'form-input inline-edit-input';
      span.replaceWith(input);
      input.focus();
      input.select();
      let debounceTimer = null;
      let saved = false;
      const commit = () => {
        if (saved) return;
        clearTimeout(debounceTimer);
        const val = input.value.trim();
        if (val && val !== task.title) {
          saved = true;
          MC.callAction(ctx, 'update_task', { id: task.id, title: val }).then(() => {
            task.title = val;
            const newSpan = document.createElement('span');
            newSpan.className = 'editable-title font-medium';
            newSpan.style.fontSize = '1.375rem';
            newSpan.title = 'Click to edit';
            newSpan.textContent = val;
            const cardTitle = document.querySelector(`.kanban-card[data-task-id="${task.id}"] .kanban-card-title .font-medium`);
            if (cardTitle) cardTitle.textContent = val;
            input.replaceWith(newSpan);
          });
        } else {
          const newSpan = document.createElement('span');
          newSpan.className = 'editable-title font-medium';
          newSpan.style.fontSize = '1.375rem';
          newSpan.title = 'Click to edit';
          newSpan.textContent = task.title;
          input.replaceWith(newSpan);
        }
      };
      input.addEventListener('input', () => {
        clearTimeout(debounceTimer);
        debounceTimer = setTimeout(commit, 3000);
      });
      input.addEventListener('blur', commit);
      input.addEventListener('keydown', e => {
        if (e.key === 'Enter') { e.preventDefault(); commit(); }
        if (e.key === 'Escape') { clearTimeout(debounceTimer); const s = document.createElement('span'); s.className = 'editable-title font-medium'; s.style.fontSize = '1.375rem'; s.title = 'Click to edit'; s.textContent = task.title; input.replaceWith(s); }
      });
    });

    // Bind description inline edit
    const descWrap = dialog.querySelector('.task-detail-description-wrap');
    descWrap.addEventListener('click', function(e) {
      if (!e.target.closest('.edit-desc-btn')) return;
      const origHTML = descWrap.innerHTML;
      const currentDesc = task.description || '';
      descWrap.innerHTML = `<div id="desc-edit-editor" data-editor data-field-name="description" data-preview-url="/api/preview-markdown" data-csrf-token="${MC.esc(ctx.csrf || '')}" data-initial-value="${MC.esc(currentDesc)}"></div>
        <div style="display:flex;gap:0.5rem;margin-top:0.375rem;">
          <button class="button button-primary button-sm" id="desc-save-btn">${MC.t('common.save', 'Save')}</button>
          <button class="button button-ghost button-sm" id="desc-cancel-btn">${MC.t('common.cancel', 'Cancel')}</button>
        </div>`;
      const editorEl = descWrap.querySelector('#desc-edit-editor');
      if (typeof window.initEditor === 'function') window.initEditor(editorEl);
      descWrap.querySelector('#desc-cancel-btn').addEventListener('click', () => {
        task.description = currentDesc;
        descWrap.innerHTML = origHTML;
      });
      descWrap.querySelector('#desc-save-btn').addEventListener('click', () => {
        const ta = descWrap.querySelector('textarea[data-field]');
        const val = ta ? ta.value.trim() : '';
        const params = val ? { id: task.id, description: val } : { id: task.id, clear_description: true };
        MC.callAction(ctx, 'update_task', params).then(() => {
          task.description = val;
          descWrap.innerHTML = val
            ? `<div class="task-detail-description markdown-body" id="desc-view">${MC.esc(val)}</div><button class="button button-ghost button-xs edit-desc-btn" title="${MC.t('tasks.edit_description', 'Edit description')}">${MC.t('common.edit', 'Edit')}</button>`
            : `<button class="button button-ghost button-xs edit-desc-btn" title="Add description">+ Add description</button>`;
          if (val && typeof window.fetchMarkdownPreview === 'function' && ctx.csrf) {
            const descEl = descWrap.querySelector('#desc-view');
            if (descEl) window.fetchMarkdownPreview(val, descEl, '/api/preview-markdown', ctx.csrf);
          }
        });
      });
    });

    // Bind comment edits
    const commentList = dialog.querySelector('.comment-list');
    if (commentList) {
      commentList.addEventListener('click', e => {
        const btn = e.target.closest('.comment-edit-btn');
        if (!btn) return;
        const commentId = parseInt(btn.dataset.commentId, 10);
        const bodyEl = dialog.querySelector(`#comment-body-${commentId}`);
        if (!bodyEl) return;
        const origText = bodyEl.textContent;
        const origHTML = bodyEl.outerHTML;
        bodyEl.outerHTML = `<div id="comment-edit-wrap-${commentId}">
          <div id="comment-edit-editor-${commentId}" data-editor data-field-name="body" data-preview-url="/api/preview-markdown" data-csrf-token="${MC.esc(ctx.csrf || '')}" data-initial-value="${MC.esc(origText)}"></div>
          <div style="display:flex;gap:0.5rem;margin-top:0.375rem;">
            <button class="button button-primary button-sm" id="comment-save-${commentId}">${MC.t('common.save', 'Save')}</button>
            <button class="button button-ghost button-sm" id="comment-cancel-${commentId}">${MC.t('common.cancel', 'Cancel')}</button>
          </div>
        </div>`;
        btn.style.display = 'none';
        const commentEditorEl = dialog.querySelector(`#comment-edit-editor-${commentId}`);
        if (typeof window.initEditor === 'function') window.initEditor(commentEditorEl);
        dialog.querySelector(`#comment-cancel-${commentId}`).addEventListener('click', () => {
          dialog.querySelector(`#comment-edit-wrap-${commentId}`).outerHTML = origHTML;
          btn.style.display = '';
        });
        dialog.querySelector(`#comment-save-${commentId}`).addEventListener('click', () => {
          const ta = dialog.querySelector(`#comment-edit-wrap-${commentId} textarea[data-field]`);
          const val = ta ? ta.value.trim() : '';
          if (!val) return;
          MC.callAction(ctx, 'update_comment', { id: commentId, body: val }).then(() => {
            dialog.querySelector(`#comment-edit-wrap-${commentId}`).outerHTML = `<div class="comment-body" id="comment-body-${commentId}">${MC.esc(val)}</div>`;
            btn.style.display = '';
            const updatedEl = dialog.querySelector(`#comment-body-${commentId}`);
            if (updatedEl && typeof window.fetchMarkdownPreview === 'function' && ctx.csrf) {
              window.fetchMarkdownPreview(val, updatedEl, '/api/preview-markdown', ctx.csrf);
            }
          });
        });
      });
    }

    // Initialise new-comment editor (built dynamically, no textarea yet in DOM)
    const commentEditorContainer = dialog.querySelector('#detail-comment-editor');
    if (commentEditorContainer && typeof window.initEditor === 'function') {
      window.initEditor(commentEditorContainer);
    }

    // Bind comment submit
    dialog.querySelector('#detail-comment-submit').addEventListener('click', () => {
      const ta = dialog.querySelector('#detail-comment-editor textarea[data-field]');
      const body = ta ? ta.value.trim() : '';
      if (!body) return;
      const commentParams = { task_id: task.id, body: body, author_name: ctx.username || 'Anonymous' };
      if (ctx.userId) commentParams.author_id = ctx.userId;
      MC.callAction(ctx, 'create_comment', commentParams)
        .then(() => openTaskDetail(taskId, ctx));
    });

    // Copy link
    dialog.querySelector('#detail-copy-link').addEventListener('click', () => {
      const linkUrl = new URL(window.location.href);
      linkUrl.searchParams.set('task', taskId);
      navigator.clipboard.writeText(linkUrl.toString()).then(() => MC.showToast('Link copied'));
    });

    // Close — clear task param from URL, reload if timer state changed
    function closeDetail() {
      const activeInput = dialog.querySelector('.inline-edit-input');
      if (activeInput) activeInput.blur();
      dialog.close();
      const cleanUrl = new URL(window.location.href);
      cleanUrl.searchParams.delete('task');
      history.replaceState(null, '', cleanUrl.toString());
      if (dialog._timerChanged) {
        dialog._timerChanged = false;
        MC.reloadPage();
      }
    }
    dialog._closeDetail = closeDetail;
    dialog.querySelector('#detail-close').addEventListener('click', closeDetail);

    // Load tracked time from pomodoro-plugin
    const trackedEl = dialog.querySelector('#detail-tracked-time');
    if (trackedEl) {
      fetch('/p/pomodoro-plugin/action/list_sessions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': ctx.csrf },
        body: JSON.stringify({ task_id: task.id, limit: 50 }),
      }).then(r => r.json()).catch(() => null).then(sessData => {
        const allSessions = (sessData && sessData.sessions) || [];
        const activeSess = allSessions.find(s => !s.ended_at) || null;
        const sessions = allSessions.filter(s => s.ended_at);
        let th = '<h3 class="text-sm font-semibold" style="margin:0 0 0.5rem;">Tracked Time</h3>';
        // Active timer for this task
        if (activeSess) {
          const isPaused = !!activeSess.paused_at;
          const pausedSec = activeSess.total_paused_sec || 0;
          const elapsedSec = isPaused
            ? Math.max(0, Math.floor((new Date(activeSess.paused_at) - new Date(activeSess.started_at)) / 1000) - pausedSec)
            : Math.max(0, Math.floor((Date.now() - new Date(activeSess.started_at).getTime()) / 1000) - pausedSec);
          const fmtTime = s => { const h = Math.floor(s/3600); const m = Math.floor((s%3600)/60); const sc = s%60; return String(h).padStart(2,'0')+':'+String(m).padStart(2,'0')+':'+String(sc).padStart(2,'0'); };
          th += `<div style="display:flex;align-items:center;gap:0.75rem;margin-bottom:0.75rem;">`;
          th += `<span class="timer-elapsed" style="font-size:1.25rem;font-weight:600;font-variant-numeric:tabular-nums;${isPaused ? 'color:hsl(40 90% 60%)' : 'color:hsl(var(--primary))'}" ${isPaused ? '' : `data-started="${activeSess.started_at}" data-paused-sec="${pausedSec}"`}>${fmtTime(elapsedSec)}</span>`;
          th += `<span class="badge badge-${isPaused ? 'pause' : 'task'}">${isPaused ? 'paused' : 'tracking'}</span>`;
          if (isPaused) th += `<button class="button button-primary button-sm" id="detail-timer-resume">Resume</button>`;
          else th += `<button class="button button-ghost button-sm" id="detail-timer-pause">Pause</button>`;
          th += `<button class="button button-primary button-sm" id="detail-timer-stop">Stop</button>`;
          th += `</div>`;
        } else {
          th += `<div style="margin-bottom:0.75rem;">`;
          th += `<button class="button button-ghost button-sm" id="detail-timer-start">▶ Start Timer</button>`;
          th += `</div>`;
        }
        // Session history for this task
        if (sessions.length > 0) {
          const fmtDur = s => { if (s >= 3600) return Math.floor(s/3600)+'h '+Math.floor((s%3600)/60)+'m'; if (s >= 60) return Math.floor(s/60)+'m'; return s+'s'; };
          const fmtDt = iso => { const d = new Date(iso); return d.toLocaleDateString('en-US',{month:'short',day:'numeric'})+', '+d.toLocaleTimeString('en-US',{hour:'2-digit',minute:'2-digit',hour12:false}); };
          let totalNet = 0;
          const completedSessions = sessions.filter(s => s.ended_at);
          for (const s of completedSessions) {
            const gross = Math.max(0, Math.floor((new Date(s.ended_at) - new Date(s.started_at)) / 1000));
            totalNet += Math.max(0, gross - (s.total_paused_sec || 0));
          }
          th += `<details class="detail-tracked-spoiler" style="font-size:0.8125rem;">`;
          th += `<summary style="cursor:pointer;color:hsl(var(--muted-foreground));margin-bottom:0.375rem;list-style:none;"><span class="detail-tracked-icon"></span> History — ${fmtDur(totalNet)} total (${completedSessions.length} session${completedSessions.length !== 1 ? 's' : ''})</summary>`;
          for (const s of completedSessions) {
            const gross = Math.max(0, Math.floor((new Date(s.ended_at) - new Date(s.started_at)) / 1000));
            const net = Math.max(0, gross - (s.total_paused_sec || 0));
            const cancelled = s.interrupted ? ' <span class="text-muted-foreground">(cancelled)</span>' : '';
            th += `<div style="display:flex;gap:0.75rem;padding:0.25rem 0;border-bottom:1px solid hsl(var(--border));">`;
            th += `<span style="min-width:5rem;">${fmtDt(s.started_at)}</span>`;
            th += `<strong>${fmtDur(net)}</strong>${cancelled}`;
            th += `</div>`;
          }
          th += `</details>`;
        } else if (!activeSess) {
          th += `<p class="text-muted-foreground text-sm">${MC.t('pomodoro.no_tracked', 'No tracked time yet.')}</p>`;
        }
        trackedEl.innerHTML = th;
        // Bind timer buttons
        const pomodoroUrl = '/p/pomodoro-plugin/action/';
        const timerFetch = (tool, params) => fetch(pomodoroUrl + tool, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': ctx.csrf },
          body: JSON.stringify(params),
        }).then(r => r.json()).then(() => { dialog._timerChanged = true; openTaskDetail(task.id, ctx); });
        const startBtn = trackedEl.querySelector('#detail-timer-start');
        if (startBtn) startBtn.addEventListener('click', () => {
          const params = { session_type: 'task', task_id: task.id };
          if (task.project_id) params.project_id = task.project_id;
          timerFetch('start_session', params);
        });
        const stopBtn = trackedEl.querySelector('#detail-timer-stop');
        if (stopBtn) stopBtn.addEventListener('click', () => timerFetch('stop_session', { task_id: task.id }));
        const pauseBtn = trackedEl.querySelector('#detail-timer-pause');
        if (pauseBtn) pauseBtn.addEventListener('click', () => timerFetch('pause_session', { task_id: task.id }));
        const resumeBtn = trackedEl.querySelector('#detail-timer-resume');
        if (resumeBtn) resumeBtn.addEventListener('click', () => timerFetch('resume_session', { task_id: task.id }));
        // Live timer update (clear previous interval)
        if (dialog._detailTimerInterval) clearInterval(dialog._detailTimerInterval);
        const liveEl = trackedEl.querySelector('.timer-elapsed[data-started]');
        if (liveEl) {
          const ps = parseInt(liveEl.dataset.pausedSec || '0', 10);
          const fmtTime = s => { const h = Math.floor(s/3600); const m = Math.floor((s%3600)/60); const sc = s%60; return String(h).padStart(2,'0')+':'+String(m).padStart(2,'0')+':'+String(sc).padStart(2,'0'); };
          dialog._detailTimerInterval = setInterval(() => { liveEl.textContent = fmtTime(Math.max(0, Math.floor((Date.now() - new Date(liveEl.dataset.started).getTime()) / 1000) - ps)); }, 1000);
        }
      });
    }
  }).catch(() => {});
}

function openBoardSettings(ctx, opts) {
  opts = opts || {};
  let dialog = document.getElementById('mc-board-settings');
  if (!dialog) {
    dialog = document.createElement('dialog');
    dialog.id = 'mc-board-settings';
    dialog.className = 'mc-dialog board-settings-dialog';
    document.body.appendChild(dialog);
    bindBackdropClose(dialog, () => {
      dialog.close();
      MC.reloadPage();
    });
  }

  let activeTab = 'columns';
  renderSettingsDialog();

  function renderSettingsDialog() {
    Promise.all([
      MC.callAction(ctx, 'list_board_columns', ctx.projectId ? { project_id: ctx.projectId } : {}),
      MC.callAction(ctx, 'list_task_types', ctx.projectId ? { project_id: ctx.projectId } : {})
    ]).then(([colData, typeData]) => {
      const columns = colData.columns || [];
      const types = typeData.types || [];

      let html = '<div class="settings-dialog-header">';
      html += '<span class="font-medium" style="font-size:1.125rem;">Board Settings</span>';
      html += '<button class="button button-ghost button-xs settings-close">&times;</button>';
      html += '</div>';

      html += '<div class="settings-tabs">';
      html += `<button class="settings-tab${activeTab === 'columns' ? ' active' : ''}" data-tab="columns">Columns</button>`;
      html += `<button class="settings-tab${activeTab === 'types' ? ' active' : ''}" data-tab="types">Task Types</button>`;
      html += '</div>';

      html += '<div class="settings-body">';
      if (activeTab === 'columns') {
        html += renderColumnsTab(columns);
      } else {
        html += renderTypesTab(types);
      }
      html += '</div>';

      dialog.innerHTML = html;
      dialog._mousedownOnBackdrop = false;
      dialog.showModal();
      bindSettingsEvents(dialog, columns, types);
      // Auto-expand "Add column" form when requested
      if (opts.focusAddColumn && activeTab === 'columns') {
        const trigger = dialog.querySelector('#settings-add-col-trigger');
        const form = dialog.querySelector('#settings-add-col-form');
        if (trigger && form) {
          trigger.style.display = 'none';
          form.style.display = 'flex';
          const nameInput = dialog.querySelector('#settings-add-col-name');
          if (nameInput) setTimeout(() => nameInput.focus(), 50);
        }
        opts.focusAddColumn = false; // only on first render
      }
    });
  }

  function renderColumnsTab(columns) {
    let html = '<div class="settings-list" id="settings-columns-list">';
    columns.sort((a, b) => a.position - b.position);
    columns.forEach((col) => {
      const isDefault = col.is_default;
      html += `<div class="settings-item" draggable="true" data-col-id="${col.id}" data-col-pos="${col.position}">`;
      html += `<span class="drag-handle" title="Drag to reorder">&#9776;</span>`;
      html += `<input type="color" class="color-dot" value="${MC.esc(col.color || '#6b7280')}" data-col-id="${col.id}" title="Change color">`;
      html += `<input type="text" class="inline-label" value="${MC.esc(col.label)}" data-col-id="${col.id}" data-field="label">`;
      if (isDefault) {
        html += `<span class="lock-icon" title="Default column">&#128274;</span>`;
      } else {
        html += `<button class="button button-ghost button-xs settings-delete-col" data-col-id="${col.id}" title="${MC.t('tasks.delete_column', 'Delete column')}">&#128465;</button>`;
      }
      html += '</div>';
    });
    html += '</div>';
    html += '<div class="settings-add-row" id="settings-add-col-row">';
    html += '<span class="settings-add-trigger" id="settings-add-col-trigger">+ Add column</span>';
    html += '<div class="settings-add-form" id="settings-add-col-form" style="display:none;">';
    html += '<input type="text" class="form-input" id="settings-add-col-name" placeholder="column_name (slug)">';
    html += '<input type="text" class="form-input" id="settings-add-col-label" placeholder="Display Label">';
    html += '<input type="color" class="color-dot" id="settings-add-col-color" value="#6b7280">';
    html += '<button class="button button-primary button-sm" id="settings-add-col-submit">Add</button>';
    html += '<button class="button button-ghost button-sm" id="settings-add-col-cancel">Cancel</button>';
    html += '</div></div>';
    return html;
  }

  function renderTypesTab(types) {
    let html = '<div class="settings-list" id="settings-types-list">';
    types.forEach(tt => {
      html += `<div class="settings-item" data-type-id="${tt.id}">`;
      html += `<input type="color" class="color-dot" value="${MC.esc(tt.color || '#6b7280')}" data-type-id="${tt.id}" title="Change color">`;
      const iconName = tt.icon || 'star';
      html += `<button class="button button-ghost button-xs icon-trigger" data-type-id="${tt.id}" data-icon="${MC.esc(iconName)}" title="Change icon"><svg class="icon" width="16" height="16"><use href="/static/icons/sprite.svg#${MC.esc(iconName)}"></use></svg></button>`;
      html += `<input type="text" class="inline-label" value="${MC.esc(tt.label)}" data-type-id="${tt.id}" data-field="label">`;
      html += `<button class="button button-ghost button-xs settings-delete-type" data-type-id="${tt.id}" title="${MC.t('tasks.delete_type', 'Delete type')}">&#128465;</button>`;
      html += '</div>';
    });
    html += '</div>';
    html += '<div class="settings-add-row" id="settings-add-type-row">';
    html += '<span class="settings-add-trigger" id="settings-add-type-trigger">+ Add type</span>';
    html += '<div class="settings-add-form" id="settings-add-type-form" style="display:none;">';
    html += '<input type="text" class="form-input" id="settings-add-type-name" placeholder="type_name (slug)">';
    html += '<input type="text" class="form-input" id="settings-add-type-label" placeholder="Display Label">';
    html += '<input type="color" class="color-dot" id="settings-add-type-color" value="#6b7280">';
    html += '<button class="button button-primary button-sm" id="settings-add-type-submit">Add</button>';
    html += '<button class="button button-ghost button-sm" id="settings-add-type-cancel">Cancel</button>';
    html += '</div></div>';
    return html;
  }

  function bindSettingsEvents(dlg, columns, types) {
    dlg.querySelector('.settings-close').addEventListener('click', () => { dlg.close(); MC.reloadPage(); });

    dlg.querySelectorAll('.settings-tab').forEach(tab => {
      tab.addEventListener('click', () => {
        activeTab = tab.dataset.tab;
        renderSettingsDialog();
      });
    });

    if (activeTab === 'columns') {
      bindColumnsEvents(dlg, columns);
    } else {
      bindTypesEvents(dlg, types);
    }
  }

  function bindColumnsEvents(dlg, columns) {
    dlg.querySelectorAll('.inline-label[data-col-id]').forEach(input => {
      const origVal = input.value;
      input.addEventListener('blur', () => {
        if (input.value !== origVal && input.value.trim()) {
          MC.callAction(ctx, 'update_board_column', { id: Number(input.dataset.colId), label: input.value.trim() })
            .then(() => MC.showToast('Column updated'));
        }
      });
      input.addEventListener('keydown', e => { if (e.key === 'Enter') input.blur(); });
    });

    dlg.querySelectorAll('.color-dot[data-col-id]').forEach(input => {
      input.addEventListener('change', () => {
        MC.callAction(ctx, 'update_board_column', { id: Number(input.dataset.colId), color: input.value })
          .then(() => MC.showToast('Color updated'));
      });
    });

    dlg.querySelectorAll('.settings-delete-col').forEach(btn => {
      btn.addEventListener('click', () => {
        MC.confirm({ title: MC.t('tasks.delete_column', 'Delete column'), message: MC.t('tasks.delete_column_confirm', 'Delete this column? Tasks in it must be moved first.') }).then(ok => {
          if (!ok) return;
          MC.callAction(ctx, 'delete_board_column', { id: Number(btn.dataset.colId) })
            .then(() => { MC.showToast('Column deleted'); renderSettingsDialog(); })
            .catch(() => MC.showToast('Cannot delete: column has tasks or is default', 'error'));
        });
      });
    });

    const list = dlg.querySelector('#settings-columns-list');
    if (list) {
      let dragItem = null;
      list.querySelectorAll('.settings-item[data-col-id]').forEach(item => {
        item.addEventListener('dragstart', e => {
          dragItem = item;
          item.style.opacity = '0.4';
          e.dataTransfer.effectAllowed = 'move';
        });
        item.addEventListener('dragend', () => { item.style.opacity = '1'; dragItem = null; });
        item.addEventListener('dragover', e => { e.preventDefault(); e.dataTransfer.dropEffect = 'move'; });
        item.addEventListener('drop', e => {
          e.preventDefault();
          if (!dragItem || dragItem === item) return;
          const items = [...list.querySelectorAll('.settings-item[data-col-id]')];
          const fromIdx = items.indexOf(dragItem);
          const toIdx = items.indexOf(item);
          if (fromIdx < toIdx) {
            item.after(dragItem);
          } else {
            item.before(dragItem);
          }
          const reordered = [...list.querySelectorAll('.settings-item[data-col-id]')];
          const updates = reordered.map((el, i) => {
            const colId = Number(el.dataset.colId);
            const oldPos = Number(el.dataset.colPos);
            if (oldPos !== i) {
              return MC.callAction(ctx, 'update_board_column', { id: colId, position: i });
            }
            return Promise.resolve();
          });
          Promise.all(updates)
            .then(() => MC.showToast('Order updated'))
            .catch(err => MC.showToast('Failed to save order: ' + (err && err.message ? err.message : 'server error'), 'error'));
        });
      });
    }

    const addTrigger = dlg.querySelector('#settings-add-col-trigger');
    const addForm = dlg.querySelector('#settings-add-col-form');
    if (addTrigger && addForm) {
      addTrigger.addEventListener('click', () => { addTrigger.style.display = 'none'; addForm.style.display = 'flex'; });
      dlg.querySelector('#settings-add-col-cancel').addEventListener('click', () => { addForm.style.display = 'none'; addTrigger.style.display = ''; });
      dlg.querySelector('#settings-add-col-submit').addEventListener('click', () => {
        const name = dlg.querySelector('#settings-add-col-name').value.trim();
        if (!name) return;
        const label = dlg.querySelector('#settings-add-col-label').value.trim() || name.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
        const color = dlg.querySelector('#settings-add-col-color').value;
        const colParams = { name, label, color };
        if (ctx.projectId) colParams.project_id = ctx.projectId;
        MC.callAction(ctx, 'create_board_column', colParams)
          .then(() => { MC.showToast('Column added'); renderSettingsDialog(); })
          .catch(() => MC.showToast('Failed to create column', 'error'));
      });
    }
  }

  function bindTypesEvents(dlg, types) {
    dlg.querySelectorAll('.inline-label[data-type-id]').forEach(input => {
      const origVal = input.value;
      input.addEventListener('blur', () => {
        if (input.value !== origVal && input.value.trim()) {
          MC.callAction(ctx, 'update_task_type', { id: Number(input.dataset.typeId), label: input.value.trim() })
            .then(() => MC.showToast('Type updated'));
        }
      });
      input.addEventListener('keydown', e => { if (e.key === 'Enter') input.blur(); });
    });

    dlg.querySelectorAll('.color-dot[data-type-id]').forEach(input => {
      input.addEventListener('change', () => {
        MC.callAction(ctx, 'update_task_type', { id: Number(input.dataset.typeId), color: input.value })
          .then(() => MC.showToast('Color updated'));
      });
    });

    dlg.querySelectorAll('.icon-trigger').forEach(btn => {
      btn.addEventListener('click', () => {
        let picker = dlg.querySelector('.icon-picker-dropdown');
        if (picker) { picker.remove(); return; }
        picker = document.createElement('div');
        picker.className = 'icon-picker-dropdown';
        picker.innerHTML = ICONS.map(name =>
          `<button class="icon-pick-item" data-icon="${name}" title="${name}"><svg class="icon" width="16" height="16"><use href="/static/icons/sprite.svg#${name}"></use></svg></button>`
        ).join('');
        btn.after(picker);
        picker.querySelectorAll('.icon-pick-item').forEach(item => {
          item.addEventListener('click', () => {
            const icon = item.dataset.icon;
            MC.callAction(ctx, 'update_task_type', { id: Number(btn.dataset.typeId), icon })
              .then(() => { MC.showToast('Icon updated'); renderSettingsDialog(); });
          });
        });
      });
    });

    dlg.querySelectorAll('.settings-delete-type').forEach(btn => {
      btn.addEventListener('click', () => {
        MC.confirm({ title: MC.t('tasks.delete_type', 'Delete task type'), message: MC.t('tasks.delete_type_confirm', 'Delete this task type? Tasks with this type will have their type cleared.') }).then(ok => {
          if (!ok) return;
          MC.callAction(ctx, 'delete_task_type', { id: Number(btn.dataset.typeId) })
            .then(() => { MC.showToast('Type deleted'); renderSettingsDialog(); });
        });
      });
    });

    const addTrigger = dlg.querySelector('#settings-add-type-trigger');
    const addForm = dlg.querySelector('#settings-add-type-form');
    if (addTrigger && addForm) {
      addTrigger.addEventListener('click', () => { addTrigger.style.display = 'none'; addForm.style.display = 'flex'; });
      dlg.querySelector('#settings-add-type-cancel').addEventListener('click', () => { addForm.style.display = 'none'; addTrigger.style.display = ''; });
      dlg.querySelector('#settings-add-type-submit').addEventListener('click', () => {
        const name = dlg.querySelector('#settings-add-type-name').value.trim();
        if (!name) return;
        const label = dlg.querySelector('#settings-add-type-label').value.trim() || name.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
        const color = dlg.querySelector('#settings-add-type-color').value;
        const typeParams = { name, label, color };
        if (ctx.projectId) typeParams.project_id = ctx.projectId;
        MC.callAction(ctx, 'create_task_type', typeParams)
          .then(() => { MC.showToast('Type added'); renderSettingsDialog(); })
          .catch(() => MC.showToast('Failed to create type', 'error'));
      });
    }
  }
}

// Register kanban renderer
MC.registerRenderer('kanban', renderKanban);

// Populate plugin-specific selects inside the create dialog.
// Called by main.js bindPageActions when a create dialog is opened.
MC.populateDialogSelects = function(dialog, ctx) {
  // Project selects are populated by main.js (generic for all plugins).
  // Populate status selects (default: pending, or ctx._pendingStatus if set by column "+" button)
  dialog.querySelectorAll('[data-status-select]').forEach(sel => {
    const defaultStatus = ctx._pendingStatus || 'pending';
    ctx._pendingStatus = null;
    const colParams = ctx.projectId ? { project_id: ctx.projectId } : {};
    fetch(ctx.actionUrl + 'list_board_columns', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': ctx.csrf },
      body: JSON.stringify(colParams),
    }).then(r => r.json()).then(data => {
      const cols = data.columns || [];
      for (const c of cols) {
        const opt = document.createElement('option');
        opt.value = c.name;
        opt.textContent = c.label;
        if (c.name === defaultStatus) opt.selected = true;
        sel.appendChild(opt);
      }
    }).catch(() => {});
  });
  // Populate task type selects (default: first type)
  dialog.querySelectorAll('[data-type-select]').forEach(sel => {
    const typeParams = ctx.projectId ? { project_id: ctx.projectId } : {};
    fetch(ctx.actionUrl + 'list_task_types', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': ctx.csrf },
      body: JSON.stringify(typeParams),
    }).then(r => r.json()).then(data => {
      const types = data.types || [];
      for (let i = 0; i < types.length; i++) {
        const t = types[i];
        const opt = document.createElement('option');
        opt.value = String(t.id);
        opt.textContent = t.label;
        if (i === 0) opt.selected = true;
        sel.appendChild(opt);
      }
    }).catch(() => {});
  });
};

// Handle ?task=ID deep link
MC.handleDeepLink = function(ctx) {
  const urlTask = new URLSearchParams(window.location.search).get('task');
  if (urlTask) openTaskDetail(Number(urlTask), ctx);
};

// Bind kanban-specific page actions
MC.bindKanbanActions = function(el, ctx) {
  // Fill remaining viewport height + fix column width
  const board = el.querySelector('.kanban-board');
  if (board) {
    const applyHeight = () => {
      const top = board.getBoundingClientRect().top;
      board.style.height = (window.innerHeight - top) + 'px';
    };
    applyHeight();
    window.addEventListener('resize', applyHeight);
    board.querySelectorAll('.kanban-column:not(.kanban-column-ghost)').forEach(col => {
      col.style.flex = '0 0 320px';
      col.style.width = '320px';
      col.style.minWidth = '320px';
      col.style.maxWidth = '320px';
    });
  }

  // Task detail dialog — open on card click
  el.querySelectorAll('[data-action="open-detail"]').forEach(card => {
    card.addEventListener('click', (e) => {
      if (e.target.closest('[data-action="set-status"], [data-action="delete"], [data-action="ext-action"], [data-action="call-tool"]')) return;
      const taskId = Number(card.dataset.taskId);
      if (taskId) openTaskDetail(taskId, ctx);
    });
  });
  // Board settings
  el.querySelectorAll('[data-action="open-board-settings"]').forEach(btn => {
    btn.addEventListener('click', () => openBoardSettings(ctx));
  });
  // Ghost column — open board settings with add-column focused
  el.querySelectorAll('[data-action="open-board-settings-add-col"]').forEach(btn => {
    btn.addEventListener('click', () => openBoardSettings(ctx, { focusAddColumn: true }));
  });
  // Column "+" buttons — open create dialog with pre-filled status
  el.querySelectorAll('[data-action="show-create-in-col"]').forEach(btn => {
    btn.addEventListener('click', (e) => {
      e.stopPropagation();
      ctx._pendingStatus = btn.dataset.colStatus;
      const newBtn = el.querySelector('[data-action="show-create"]');
      if (newBtn) newBtn.click();
    });
  });
  // Kanban drag-and-drop
  bindKanbanDragDrop(el, ctx);
  // Resolve project names for kanban cards
  const projectBadges = el.querySelectorAll('[data-project-id]');
  if (projectBadges.length > 0) {
    fetch('/p/projects-plugin/action/list_projects', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': ctx.csrf },
      body: JSON.stringify({ status: 'all' }),
    }).then(r => r.json()).then(data => {
      const map = {};
      (data.projects || []).forEach(p => { map[String(p.id)] = p.name; });
      projectBadges.forEach(badge => {
        const name = map[badge.dataset.projectId];
        if (name) badge.textContent = name;
      });
    }).catch(() => {});
  }
  // Populate kanban project filter buttons (only when NOT inside a project)
  const filterContainer = el.querySelector('#kanban-project-filters');
  if (filterContainer && !ctx.projectId) {
    fetch('/p/projects-plugin/action/list_projects', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': ctx.csrf },
      body: JSON.stringify({ status: 'active' }),
    }).then(r => r.json()).then(data => {
      const projects = data.projects || [];
      if (projects.length === 0) return;
      const allBtn = document.createElement('button');
      allBtn.className = 'button button-ghost button-sm filter-active';
      allBtn.textContent = 'All';
      allBtn.dataset.filterId = '';
      filterContainer.appendChild(allBtn);
      for (const p of projects) {
        const btn = document.createElement('button');
        btn.className = 'button button-ghost button-sm';
        btn.textContent = p.name;
        btn.dataset.filterId = String(p.id);
        filterContainer.appendChild(btn);
      }
      filterContainer.addEventListener('click', e => {
        const btn = e.target.closest('[data-filter-id]');
        if (!btn) return;
        filterContainer.querySelectorAll('[data-filter-id]').forEach(b => b.classList.remove('filter-active'));
        btn.classList.add('filter-active');
        const filterId = btn.dataset.filterId;
        el.querySelectorAll('.kanban-card').forEach(card => {
          if (!filterId) { card.style.display = ''; return; }
          const badge = card.querySelector('[data-project-id]');
          card.style.display = (badge && badge.dataset.projectId === filterId) ? '' : 'none';
        });
      });
    }).catch(() => {});
  }
};
