'use strict';
// Small progressive enhancements. Everything works without JS except htmx swaps.

// Unsaved-changes guard: any <form data-guard> warns before navigating away
// once a field has changed, until the form is submitted.
(function () {
  let dirty = false;
  document.addEventListener('input', (e) => {
    if (e.target.closest('form[data-guard]')) dirty = true;
  });
  document.addEventListener('submit', (e) => {
    if (e.target.matches('form[data-guard]')) dirty = false;
  });
  window.addEventListener('beforeunload', (e) => {
    if (dirty) { e.preventDefault(); e.returnValue = ''; }
  });
})();

// Send the CSRF token (from <meta name="csrf-token">) with every htmx request.
document.addEventListener('htmx:configRequest', (e) => {
  const m = document.querySelector('meta[name="csrf-token"]');
  if (m) e.detail.headers['X-CSRF-Token'] = m.content;
});

// Flash toast fades out on its own.
document.addEventListener('DOMContentLoaded', () => {
  const t = document.querySelector('[data-toast]');
  if (t) setTimeout(() => t.remove(), 3000);
});

// <form data-confirm="Question?"> asks before submitting (inline handlers are blocked by CSP).
document.addEventListener('submit', (e) => {
  const f = e.target;
  if (f.dataset && f.dataset.confirm && !window.confirm(f.dataset.confirm)) e.preventDefault();
}, true);

// ---- University search box (results come from htmx: /catalog/universities/search) ----
(function () {
  const parts = (c) => {
    const form = c.closest('form');
    return {
      q: c.querySelector('.combo-q'), id: c.querySelector('[name=university_id]'),
      name: c.querySelector('[name=university_name]'), list: c.querySelector('.combo-list'),
      other: c.querySelector('.combo-other'), otherName: c.querySelector('[name=uni_other]'),
      country: form.querySelector('[data-app-country]'),
    };
  };
  const close = (p) => { p.list.hidden = true; p.q.setAttribute('aria-expanded', 'false'); };
  const clearPick = (p) => { p.id.value = ''; p.name.value = ''; p.id.dataset.country = ''; };
  function setOther(c, on) {
    const p = parts(c);
    p.q.hidden = on; p.other.hidden = !on; close(p);
    if (on) { clearPick(p); p.otherName.value = p.q.value; p.otherName.focus(); }
    else { p.otherName.value = ''; p.q.focus(); }
  }
  document.addEventListener('htmx:afterSwap', (e) => {
    if (e.target.classList.contains('combo-list')) {
      e.target.hidden = false;
      e.target.closest('[data-combo]').querySelector('.combo-q').setAttribute('aria-expanded', 'true');
    }
  });
  document.addEventListener('click', (e) => {
    const pick = e.target.closest('[data-pick]');
    if (pick) {
      const c = pick.closest('[data-combo]'), p = parts(c);
      if (pick.dataset.pick === '__other') { setOther(c, true); return; }
      p.id.value = pick.dataset.pick; p.name.value = pick.dataset.name; p.id.dataset.country = pick.dataset.country;
      p.q.value = pick.dataset.name; close(p);
      // A list university belongs to one country: show it in the Country dropdown.
      if (p.country && pick.dataset.country) p.country.value = pick.dataset.country;
      return;
    }
    const back = e.target.closest('[data-combo-back]');
    if (back) { setOther(back.closest('[data-combo]'), false); return; }
    document.querySelectorAll('[data-combo]').forEach((c) => { if (!c.contains(e.target)) close(parts(c)); });
  });
  document.addEventListener('input', (e) => {
    if (e.target.classList.contains('combo-q')) {
      clearPick(parts(e.target.closest('[data-combo]')));
    }
    if ('digits' in e.target.dataset) {
      const v = e.target.value.replace(/\D/g, '');
      if (v !== e.target.value) e.target.value = v;
    }
  });
  // Changing the country drops a picked list university from another country.
  document.addEventListener('change', (e) => {
    if (!e.target.matches('[data-app-country]')) return;
    const c = e.target.closest('form').querySelector('[data-combo]');
    if (!c) return;
    const p = parts(c);
    if (p.id.value && p.id.dataset.country && p.id.dataset.country !== e.target.value) {
      clearPick(p); p.q.value = '';
    }
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') document.querySelectorAll('[data-combo]').forEach((c) => close(parts(c)));
    if (e.key === 'Enter' && e.target.matches('tr[data-href]')) location.href = e.target.dataset.href;
  });
})();

// "Other" consultancy shows a text box.
document.addEventListener('change', (e) => {
  if (e.target.id === 'consSel') {
    const o = document.getElementById('consOther');
    o.hidden = e.target.value !== '__other';
    if (!o.hidden) o.focus();
  }
});

// Whole table rows are clickable (links inside still work normally).
document.addEventListener('click', (e) => {
  const tr = e.target.closest('tr[data-href]');
  if (tr && !e.target.closest('a,button,input,select,label')) location.href = tr.dataset.href;
});

// Copy-to-clipboard buttons (generated passwords).
document.addEventListener('click', (e) => {
  const b = e.target.closest('[data-copy]');
  if (!b) return;
  const done = () => { const t = b.textContent; b.textContent = 'Copied'; setTimeout(() => { b.textContent = t; }, 1500); };
  if (navigator.clipboard) navigator.clipboard.writeText(b.dataset.copy).then(done, () => alert('Copy blocked. Select the password and copy it.'));
});

// Create-user form: branch only for consultants.
document.addEventListener('change', (e) => {
  if (!e.target.matches('[data-role-select]')) return;
  const f = e.target.closest('form');
  f.querySelector('[data-branch-field]').hidden = e.target.value !== 'consultant';
  f.querySelector('[data-network-hint]').hidden = e.target.value !== 'agent_network';
});

// Keep filter URLs clean: drop empty GET parameters from htmx requests.
document.addEventListener('htmx:configRequest', (e) => {
  if (e.detail.verb !== 'get') return;
  const p = e.detail.parameters;
  const keys = typeof p.keys === 'function' ? Array.from(p.keys()) : Object.keys(p);
  keys.forEach((k) => {
    const v = typeof p.get === 'function' ? p.get(k) : p[k];
    if (v === '' || v == null) { if (typeof p.delete === 'function') p.delete(k); else delete p[k]; }
  });
});

// ---- Multi-select filters ----
(function () {
  const summary = (ms) => {
    const picked = [...ms.querySelectorAll('input[type=checkbox]:checked')]
      .map((c) => c.closest('.ms-opt').textContent.trim());
    const el = ms.querySelector('[data-ms-label]');
    el.textContent = picked.length === 0 ? ms.dataset.any
      : picked.length <= 2 ? picked.join(', ') : picked[0] + ' +' + (picked.length - 1);
    el.classList.toggle('ms-on', picked.length > 0);
  };
  document.addEventListener('change', (e) => {
    const ms = e.target.closest('[data-ms]');
    if (ms && e.target.type === 'checkbox') summary(ms);
  });
  // One open at a time; close on outside click or Escape.
  document.addEventListener('click', (e) => {
    document.querySelectorAll('details[data-ms][open]').forEach((d) => {
      if (!d.contains(e.target)) d.open = false;
    });
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') document.querySelectorAll('details[data-ms][open]').forEach((d) => { d.open = false; });
  });
  // Type-to-find inside long lists.
  document.addEventListener('input', (e) => {
    if (!e.target.matches('[data-ms-search]')) return;
    const q = e.target.value.trim().toLowerCase();
    e.target.closest('.ms-panel').querySelectorAll('.ms-opt').forEach((o) => {
      o.hidden = q !== '' && !o.textContent.toLowerCase().includes(q);
    });
  });
})();

// Export-ready notices: dismiss with ×, or automatically after clicking Download.
document.addEventListener('click', (e) => {
  const x = e.target.closest('[data-dismiss]');
  if (x) { x.closest('.notice-card').remove(); return; }
  const d = e.target.closest('[data-dismiss-after]');
  if (d) setTimeout(() => d.closest('.notice-card').remove(), 500);
});
