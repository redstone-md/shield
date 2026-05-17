// global helpers for the Shield web ui.

// copyUserID copies a telegram user id to the clipboard and gives the button feedback.
function copyUserID(userId, btn) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(String(userId)).then(function () {
            if (btn) {
                btn.textContent = 'Copied!';
                btn.classList.add('btn-ham');
                setTimeout(function () {
                    btn.textContent = 'Copy ID';
                    btn.classList.remove('btn-ham');
                }, 2000);
            }
        }).catch(function () {
            if (btn) {
                btn.textContent = 'Failed to copy';
                setTimeout(function () { btn.textContent = 'Copy ID'; }, 2000);
            }
        });
    } else if (btn) {
        btn.textContent = 'Failed to copy';
        setTimeout(function () { btn.textContent = 'Copy ID'; }, 2000);
    }
}

// re-initialize Alpine components inside fragments swapped in by HTMX.
document.body.addEventListener('htmx:afterSettle', function (evt) {
    if (window.Alpine && evt.detail && evt.detail.target) {
        window.Alpine.initTree(evt.detail.target);
    }
});
