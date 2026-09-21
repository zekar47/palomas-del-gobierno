/* PALOMAS DEL GOBIERNO — app.js
   Único uso de JavaScript en el sitio: vista previa en vivo de markdown.
   Todo lo demás (votos, comentarios, foro, sesión) son formularios HTML
   que funcionan sin JS y en navegadores de texto. */
(function () {
  "use strict";

  function wirePreview() {
    var btn = document.getElementById("preview-btn");
    var textarea = document.getElementById("body");
    var wrap = document.getElementById("preview-wrap");
    var out = document.getElementById("preview");
    if (!btn || !textarea || !wrap || !out) return;

    var timer = null;

    function refresh() {
      var form = new FormData();
      form.append("body", textarea.value);
      form.append("csrf", btn.getAttribute("data-csrf") || "");
      fetch("/preview", { method: "POST", body: form, credentials: "same-origin" })
        .then(function (r) { return r.text(); })
        .then(function (html) { out.innerHTML = html; })
        .catch(function () { out.textContent = "[no se pudo renderizar la vista previa]"; });
    }

    btn.addEventListener("click", function () {
      if (wrap.hidden) {
        wrap.hidden = false;
        refresh();
        btn.textContent = "OCULTAR PREVIA";
      } else {
        wrap.hidden = true;
        btn.textContent = "VISTA PREVIA";
      }
    });

    textarea.addEventListener("input", function () {
      if (wrap.hidden) return;
      clearTimeout(timer);
      timer = setTimeout(refresh, 400);
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", wirePreview);
  } else {
    wirePreview();
  }
})();
