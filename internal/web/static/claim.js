(function () {
  var pending = false;
  function claimSession() {
    var match = (location.hash || "").match(/^#claim=([A-Za-z0-9]+)$/);
    if (!match || pending) return;
    pending = true;
    var token = match[1];
    history.replaceState(null, "", location.pathname + location.search);
    fetch("/session/claim", {
      method: "POST",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
        Accept: "text/html"
      },
      body: "claim=" + encodeURIComponent(token),
      credentials: "same-origin",
      redirect: "manual"
    }).then(function () {
      window.location.replace("/");
    }).catch(function () {
      window.location.replace("/");
    });
  }
  // Opening tracker may reuse a tab already showing this sign-in page.
  window.addEventListener("hashchange", claimSession);
  claimSession();
})();
