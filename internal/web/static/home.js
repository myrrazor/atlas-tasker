(function () {
  if (!document.body?.classList.contains('home-body')) return;

  document.querySelectorAll('dialog[open]').forEach((dialog) => {
    if (!dialog.showModal) return;
    try {
      dialog.close();
      dialog.showModal();
      dialog.querySelector('input:not([type="hidden"])')?.focus();
    } catch (err) {
      console.debug('home dialog:', err);
    }
  });

  document.querySelectorAll('form[data-busy]').forEach((form) => {
    form.addEventListener('submit', (event) => {
      if (event.defaultPrevented) return;
      form.classList.add('is-busy');
      form.querySelectorAll('button[type="submit"]').forEach((button) => {
        button.disabled = true;
      });
    });
  });

})();
