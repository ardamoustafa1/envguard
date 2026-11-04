import './bootstrap';

const appName = import.meta.env.VITE_APP_NAME || 'Laravel';

document.title = `${document.title} - ${appName}`;

if (import.meta.env.DEV) {
    console.debug(`[${appName}] running in development mode`);
}
