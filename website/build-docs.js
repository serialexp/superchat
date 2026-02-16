#!/usr/bin/env node
import { readFileSync, writeFileSync, mkdirSync, readdirSync, statSync } from 'fs';
import { join, dirname, basename, relative } from 'path';
import { fileURLToPath } from 'url';
import { marked } from 'marked';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

// Configure marked
marked.setOptions({
  gfm: true,
  breaks: false,
  headerIds: true,
  mangle: false
});

// HTML template
const template = (title, content, relativePath) => `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>${title} - SuperChat Documentation</title>
    <link rel="icon" type="image/png" href="${relativePath}favicon.png">
    <link rel="stylesheet" href="${relativePath}src/docs.css">
</head>
<body>
    <header>
        <div class="container">
            <div class="header-content">
                <a href="${relativePath}index.html" class="logo">
                    <img src="${relativePath}mascot.png" alt="SuperChat" class="mascot">
                    <span>SuperChat</span>
                </a>
                <nav>
                    <a href="${relativePath}index.html">Home</a>
                    <a href="${relativePath}docs/index.html">Documentation</a>
                    <a href="https://github.com/aeolun/superchat">GitHub</a>
                </nav>
            </div>
        </div>
    </header>

    <div class="docs-layout">
        <aside class="sidebar">
            <nav class="docs-nav">
                <h3>Getting Started</h3>
                <ul>
                    <li><a href="${relativePath}docs/README.html">Overview</a></li>
                </ul>

                <h3>Operations</h3>
                <ul>
                    <li><a href="${relativePath}docs/ops/DEPLOYMENT.html">Deployment</a></li>
                    <li><a href="${relativePath}docs/ops/CONFIGURATION.html">Configuration</a></li>
                    <li><a href="${relativePath}docs/ops/SECURITY.html">Security</a></li>
                    <li><a href="${relativePath}docs/ops/MONITORING.html">Monitoring</a></li>
                    <li><a href="${relativePath}docs/ops/BACKUP_AND_RECOVERY.html">Backup & Recovery</a></li>
                </ul>

                <h3>Architecture</h3>
                <ul>
                    <li><a href="${relativePath}docs/PROTOCOL.html">Protocol Spec</a></li>
                    <li><a href="${relativePath}docs/DATA_MODEL.html">Data Model</a></li>
                    <li><a href="${relativePath}docs/MIGRATIONS.html">Migrations</a></li>
                </ul>

                <h3>Versions</h3>
                <ul>
                    <li><a href="${relativePath}docs/versions/V1.html">V1 Specification</a></li>
                    <li><a href="${relativePath}docs/versions/V2.html">V2 Specification</a></li>
                    <li><a href="${relativePath}docs/versions/V3.html">V3 Specification</a></li>
                </ul>

                <h3>Development</h3>
                <ul>
                    <li><a href="${relativePath}docs/IMPROVEMENTS_ROADMAP.html">Improvements Roadmap</a></li>
                    <li><a href="${relativePath}docs/DOCKER.html">Docker Guide</a></li>
                </ul>
            </nav>
        </aside>

        <main class="docs-content">
            <article class="markdown-body">
                ${content}
            </article>
        </main>
    </div>

    <footer>
        <div class="container">
            <p>&copy; 2026 SuperChat. Built with Go and Bubble Tea.</p>
        </div>
    </footer>
</body>
</html>`;

// Create docs index
const createDocsIndex = (outputDir) => {
  const indexContent = `# SuperChat Documentation

Welcome to the SuperChat documentation!

## Getting Started

- [Overview](README.html) - Project overview and quick start

## Operations Guides

- [Deployment](ops/DEPLOYMENT.html) - Complete deployment guide
- [Configuration](ops/CONFIGURATION.html) - Configuration reference
- [Security](ops/SECURITY.html) - Security hardening guide
- [Monitoring](ops/MONITORING.html) - Monitoring and observability
- [Backup & Recovery](ops/BACKUP_AND_RECOVERY.html) - Backup strategies

## Architecture Documentation

- [Protocol Specification](PROTOCOL.html) - Binary protocol details
- [Data Model](DATA_MODEL.html) - Database schema
- [Migrations](MIGRATIONS.html) - Migration system

## Version Specifications

- [V1 Specification](versions/V1.html) - V1 features and implementation
- [V2 Specification](versions/V2.html) - V2 features and status
- [V3 Specification](versions/V3.html) - V3 planned features

## Development

- [Improvements Roadmap](IMPROVEMENTS_ROADMAP.html) - Planned improvements
- [Docker Guide](DOCKER.html) - Docker deployment`;

  const html = marked.parse(indexContent);
  const fullHtml = template('Documentation', html, '../');
  writeFileSync(join(outputDir, 'index.html'), fullHtml);
};

// Recursively process markdown files
function processDirectory(inputDir, outputDir, rootDir) {
  const files = readdirSync(inputDir);

  for (const file of files) {
    const inputPath = join(inputDir, file);
    const stat = statSync(inputPath);

    if (stat.isDirectory()) {
      // Skip node_modules, .git, etc
      if (file.startsWith('.') || file === 'node_modules') continue;

      const newOutputDir = join(outputDir, file);
      mkdirSync(newOutputDir, { recursive: true });
      processDirectory(inputPath, newOutputDir, rootDir);
    } else if (file.endsWith('.md')) {
      const content = readFileSync(inputPath, 'utf-8');
      const html = marked.parse(content);

      // Calculate relative path to root
      const depth = relative(outputDir, rootDir).split('/').filter(p => p === '..').length;
      const relativePath = depth > 0 ? '../'.repeat(depth) : './';

      // Extract title from first h1 or use filename
      const titleMatch = content.match(/^#\s+(.+)$/m);
      const title = titleMatch ? titleMatch[1] : basename(file, '.md');

      const fullHtml = template(title, html, relativePath);
      const outputPath = join(outputDir, file.replace('.md', '.html'));
      writeFileSync(outputPath, fullHtml);
      console.log(`Generated: ${relative(rootDir, outputPath)}`);
    }
  }
}

// Main
const docsInputDir = join(__dirname, '..', 'docs');
const docsOutputDir = join(__dirname, 'public', 'docs');

// Create output directory
mkdirSync(docsOutputDir, { recursive: true });

// Create docs index
createDocsIndex(docsOutputDir);
console.log('Generated: docs/index.html');

// Process all markdown files
processDirectory(docsInputDir, docsOutputDir, docsOutputDir);

console.log('\nDocs build complete!');
