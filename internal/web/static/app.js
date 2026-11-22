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
