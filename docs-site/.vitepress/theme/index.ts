import DefaultTheme from 'vitepress/theme';
import './custom.css';

let d2Promise: Promise<{ compile: Function; render: Function }> | null = null;

export default {
  extends: DefaultTheme,
  enhanceApp() {
    if (typeof window !== 'undefined') {
      const runDiagramEnhancers = () => {
        void initD2Diagrams();
        setTimeout(initMermaidLightbox, 100);
      };

      const observer = new MutationObserver(() => {
        runDiagramEnhancers();
      });
      observer.observe(document.documentElement, { childList: true, subtree: true });

      if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', runDiagramEnhancers, { once: true });
      } else {
        runDiagramEnhancers();
      }

      window.addEventListener('load', () => {
        runDiagramEnhancers();
      });

      document.addEventListener('vitepress:page-loaded', () => {
        runDiagramEnhancers();
      });
    }
  },
};

async function getD2() {
  if (!d2Promise) {
    d2Promise = import('@terrastruct/d2').then(({ D2 }) => new D2());
  }
  return d2Promise;
}

async function initD2Diagrams() {
  const blocks = document.querySelectorAll('pre code:not(.d2-processed)');

  let d2: { compile: Function; render: Function };
  try {
    d2 = await getD2();
  } catch (error) {
    console.error('Failed to load D2 runtime', error);
    return;
  }

  for (const block of blocks) {
    const source = block.textContent ?? '';
    const pre = block.closest('pre');
    if (!pre) continue;

    const preClass = pre.className;
    const codeClass = block.className;
    const containerClass = pre.parentElement?.className ?? '';
    const trimmed = source.trimStart();
    const isD2Fence = preClass.includes('language-d2')
      || codeClass.includes('language-d2')
      || containerClass.includes('language-d2');
    const hasD2Marker = trimmed.startsWith('%%d2');
    const isTxtFence = preClass.includes('language-txt')
      || preClass.includes('language-text')
      || codeClass.includes('language-txt')
      || codeClass.includes('language-text')
      || containerClass.includes('language-txt')
      || containerClass.includes('language-text');
    const isTxtD2 = isTxtFence && hasD2Marker;
    const looksLikeD2 = hasD2Marker || (isD2Fence && /->|<->|:\s*\{/.test(trimmed));
    if (!isD2Fence && !isTxtD2 && !looksLikeD2) continue;

    block.classList.add('d2-processed');
    const d2Source = hasD2Marker ? trimmed.replace(/^%%d2\s*\n?/, '') : source;

    try {
      const result = await d2.compile(d2Source, { layout: 'elk' });
      let rendered: unknown = await d2.render(result.diagram, result.renderOptions);
      for (let i = 0; i < 2; i += 1) {
        if (
          rendered
          && typeof rendered === 'object'
          && 'diagram' in (rendered as Record<string, unknown>)
          && 'renderOptions' in (rendered as Record<string, unknown>)
        ) {
          const next = rendered as { diagram: unknown; renderOptions: unknown };
          rendered = await d2.render(next.diagram, next.renderOptions);
          continue;
        }
        break;
      }

      const objectSvg = rendered && typeof rendered === 'object'
        ? Object.values(rendered as Record<string, unknown>)
          .find((v) => typeof v === 'string' && v.includes('<svg'))
        : null;
      const renderedRecord = rendered && typeof rendered === 'object'
        ? (rendered as Record<string, unknown>)
        : null;
      const svg = typeof rendered === 'string'
        ? rendered
        : (renderedRecord && typeof renderedRecord.svg === 'string'
          ? renderedRecord.svg
          : (typeof objectSvg === 'string' ? objectSvg : ''));
      if (!svg) {
        throw new Error(`Unexpected D2 render result type: ${typeof rendered}`);
      }
      const wrapper = document.createElement('div');
      wrapper.className = 'd2-diagram';
      wrapper.innerHTML = svg;
      pre.replaceWith(wrapper);
    } catch (error) {
      console.error('Failed to render D2 diagram', error);
    }
  }
}

function initMermaidLightbox() {
  const mermaidDivs = document.querySelectorAll('.mermaid:not(.mermaid-processed)');

  mermaidDivs.forEach((div) => {
    div.classList.add('mermaid-processed');

    const container = document.createElement('div');
    container.className = 'mermaid-wrapper';
    div.parentNode?.insertBefore(container, div);
    container.appendChild(div);

    const hint = document.createElement('button');
    hint.className = 'mermaid-expand-hint';
    hint.textContent = '↗ Expand';
    hint.setAttribute('aria-label', 'Expand diagram');
    container.appendChild(hint);

    hint.addEventListener('click', (e) => {
      e.stopPropagation();
      openLightbox(div.cloneNode(true) as HTMLElement);
    });
  });
}

function openLightbox(chartEl: HTMLElement) {
  const overlay = document.createElement('div');
  overlay.className = 'mermaid-lightbox-overlay';

  const lightbox = document.createElement('div');
  lightbox.className = 'mermaid-lightbox';

  const closeBtn = document.createElement('button');
  closeBtn.className = 'mermaid-lightbox-close';
  closeBtn.textContent = '✕';
  closeBtn.setAttribute('aria-label', 'Close');

  const title = document.createElement('div');
  title.className = 'mermaid-lightbox-title';
  title.textContent = 'Diagram - Click outside or press ESC to close';

  lightbox.appendChild(title);
  lightbox.appendChild(chartEl);
  lightbox.appendChild(closeBtn);
  overlay.appendChild(lightbox);
  document.body.appendChild(overlay);

  // Trigger animation
  requestAnimationFrame(() => {
    overlay.classList.add('active');
  });

  const close = () => {
    overlay.classList.remove('active');
    setTimeout(() => overlay.remove(), 300);
  };

  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) close();
  });
  closeBtn.addEventListener('click', close);
  document.addEventListener('keydown', function escHandler(e) {
    if (e.key === 'Escape') {
      close();
      document.removeEventListener('keydown', escHandler);
    }
  });
}