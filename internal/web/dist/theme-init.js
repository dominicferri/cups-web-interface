// Sets the theme before the first paint. It is a separate file because the
// server's Content-Security-Policy (script-src 'self') blocks inline scripts.
(function() {
  try {
    var t = localStorage.getItem('theme');
    if (t === 'dark') {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }
  } catch (_) {}
})();
