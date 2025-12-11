// GitHub repository info - update this with your actual repo
const GITHUB_REPO = 'aeolun/superchat';

// Platform configurations
const PLATFORMS = [
    { name: 'Linux', arch: 'x86_64', tuiPattern: /^superchat-linux-amd64/, guiPattern: /^superchat-desktop-linux-amd64/, serverPattern: /^superchat-server-linux-amd64/ },
    { name: 'Linux', arch: 'ARM64', tuiPattern: /^superchat-linux-arm64/, guiPattern: null, serverPattern: /^superchat-server-linux-arm64/ },
    { name: 'macOS', arch: 'Universal', tuiPattern: /^superchat-darwin-(amd64|arm64)/, guiPattern: /^superchat-desktop-darwin-universal/, serverPattern: /^superchat-server-darwin-(amd64|arm64)/ },
    { name: 'Windows', arch: 'x86_64', tuiPattern: /^superchat-windows-amd64/, guiPattern: /^superchat-desktop-windows-amd64/, serverPattern: /^superchat-server-windows-amd64/ },
    { name: 'FreeBSD', arch: 'x86_64', tuiPattern: /^superchat-freebsd-amd64/, guiPattern: null, serverPattern: /^superchat-server-freebsd-amd64/ }
];

async function fetchLatestRelease() {
    try {
        const response = await fetch(`https://api.github.com/repos/${GITHUB_REPO}/releases/latest`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        return await response.json();
    } catch (error) {
        console.error('Failed to fetch releases:', error);
        return null;
    }
}

function createDownloadButton(asset, className, label, title) {
    const btn = document.createElement('a');
    btn.className = `download-btn ${className}`;
    btn.textContent = label;
    if (title) btn.title = title;

    if (asset) {
        btn.href = asset.browser_download_url;
    } else {
        btn.classList.add('disabled');
        btn.title = 'Not available for this platform';
        btn.addEventListener('click', (e) => e.preventDefault());
    }

    return btn;
}

function createDownloadCard(platform, tuiAsset, guiAsset, serverAsset) {
    const card = document.createElement('div');
    card.className = 'download-card';

    const platformName = document.createElement('div');
    platformName.className = 'platform';
    platformName.textContent = platform.name;

    const archName = document.createElement('div');
    archName.className = 'arch';
    archName.textContent = platform.arch;

    const buttons = document.createElement('div');
    buttons.className = 'download-buttons';

    buttons.appendChild(createDownloadButton(tuiAsset, 'tui', 'TUI', 'Terminal User Interface'));
    buttons.appendChild(createDownloadButton(guiAsset, 'gui', 'GUI', 'Graphical User Interface'));
    buttons.appendChild(createDownloadButton(serverAsset, 'server', 'Server', 'Server binary'));

    card.appendChild(platformName);
    card.appendChild(archName);
    card.appendChild(buttons);

    return card;
}

async function populateDownloads() {
    const container = document.getElementById('download-links');

    const release = await fetchLatestRelease();

    if (!release || !release.assets || release.assets.length === 0) {
        container.innerHTML = `
            <p>No releases available yet. Check <a href="https://github.com/${GITHUB_REPO}/releases">GitHub Releases</a> for updates.</p>
        `;
        return;
    }

    const cards = [];

    for (const platform of PLATFORMS) {
        const tuiAsset = release.assets.find(a => platform.tuiPattern && platform.tuiPattern.test(a.name));
        const guiAsset = release.assets.find(a => platform.guiPattern && platform.guiPattern.test(a.name));
        const serverAsset = release.assets.find(a => platform.serverPattern.test(a.name));

        if (tuiAsset || guiAsset || serverAsset) {
            cards.push(createDownloadCard(platform, tuiAsset, guiAsset, serverAsset));
        }
    }

    if (cards.length === 0) {
        container.innerHTML = `
            <p>No binary downloads found. Visit <a href="https://github.com/${GITHUB_REPO}/releases/latest">GitHub Releases</a> to download.</p>
        `;
    } else {
        container.innerHTML = '';
        cards.forEach(card => container.appendChild(card));
    }
}

// Initialize
populateDownloads();
