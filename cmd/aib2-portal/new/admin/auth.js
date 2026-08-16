// Common authentication module for AIB admin pages

const API_BASE = '/admin/api';
let token = sessionStorage.getItem('admin_token');

// Verify token on page load
async function verifyAuth() {
    if (!token) {
        redirectToLogin();
        return false;
    }

    try {
        const response = await fetch(window.location.pathname, {
            method: 'GET',
            headers: {
                'Authorization': 'Bearer ' + token
            }
        });

        // If we get 401, redirect to login
        if (response.status === 401) {
            clearAuth();
            redirectToLogin();
            return false;
        }

        return true;
    } catch (err) {
        console.error('Auth verification failed:', err);
        clearAuth();
        redirectToLogin();
        return false;
    }
}

function redirectToLogin() {
    // Preserve current page for redirect after login
    sessionStorage.setItem('admin_redirect', window.location.pathname);
    window.location.href = '/admin/login.html';
}

function clearAuth() {
    sessionStorage.removeItem('admin_token');
    sessionStorage.removeItem('admin_logged');
    sessionStorage.removeItem('admin_redirect');
}

async function logout() {
    try {
        await fetch(API_BASE + '/logout', {
            method: 'POST',
            headers: {
                'Authorization': 'Bearer ' + token,
                'Content-Type': 'application/json'
            }
        });
    } catch (err) {
        console.error('Logout error:', err);
    }
    clearAuth();
    window.location.href = '/admin/login.html';
}

// Auto-verify on page load
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', verifyAuth);
} else {
    verifyAuth();
}

// Export for use in other scripts
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { verifyAuth, logout, clearAuth };
}
