// Inline Atlas icons. Thin-stroke, rounded, no CDN. star-filled = the one
// allowed filled glyph (gold). `cls` lets callers style stroke via CSS.
const STROKE =
  'fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"';

const P: Record<string, string> = {
  'graduation-cap': `<path ${STROKE} d="M22 10 12 5 2 10l10 5 10-5Z"/><path ${STROKE} d="M6 12v5c0 1 3 3 6 3s6-2 6-3v-5"/>`,
  'list-checks': `<path ${STROKE} d="m3 5 2 2 3-3"/><path ${STROKE} d="M11 6h10M11 12h10M11 18h10"/><path ${STROKE} d="m3 11 2 2 3-3M4 17v3h3"/>`,
  'book-open': `<path ${STROKE} d="M12 7v13M12 7C10.5 5.5 8 5 3 5v13c5 0 7.5.5 9 2 1.5-1.5 4-2 9-2V5c-5 0-7.5.5-9 2Z"/>`,
  'compass': `<circle ${STROKE} cx="12" cy="12" r="9"/><path ${STROKE} d="m15.5 8.5-2 5-5 2 2-5 5-2Z"/>`,
  'sun': `<circle ${STROKE} cx="12" cy="12" r="4"/><path ${STROKE} d="M12 2v2M12 20v2M4 12H2M22 12h-2M5 5l1.5 1.5M17.5 17.5 19 19M19 5l-1.5 1.5M6.5 17.5 5 19"/>`,
  'moon': `<path ${STROKE} d="M21 12.8A8.5 8.5 0 1 1 11.2 3a6.6 6.6 0 0 0 9.8 9.8Z"/>`,
  'copy': `<rect ${STROKE} x="9" y="9" width="11" height="11" rx="2"/><path ${STROKE} d="M5 15V5a2 2 0 0 1 2-2h8"/>`,
  'arrow-right': `<path ${STROKE} d="M5 12h14M13 6l6 6-6 6"/>`,
  'external': `<path ${STROKE} d="M14 5h5v5M19 5l-8 8M12 5H7a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-5"/>`,
  'star-filled': `<path fill="currentColor" d="M12 2.5 14 9l6.5.4-5 4.2 1.7 6.4L12 16.6 6.8 20l1.7-6.4-5-4.2L10 9Z"/>`,
  // Capabilities grid (screen 1a)
  'layers': `<path ${STROKE} d="M12 2 3 7l9 5 9-5-9-5Z"/><path ${STROKE} d="M3 12l9 5 9-5M3 17l9 5 9-5"/>`,
  'providers': `<rect ${STROKE} x="3" y="4" width="18" height="7" rx="1.6"/><rect ${STROKE} x="3" y="13" width="18" height="7" rx="1.6"/><path ${STROKE} d="M7 7.5h.01M7 16.5h.01"/>`,
  'activity': `<path ${STROKE} d="M3 12h4l2 6 4-14 2 8h6"/>`,
  'link': `<path ${STROKE} d="M14.7 6.3a4 4 0 0 1 0 5.6l-3 3a4 4 0 0 1-5.6-5.6l1.5-1.5"/><path ${STROKE} d="M9.3 17.7a4 4 0 0 1 0-5.6l3-3a4 4 0 0 1 5.6 5.6l-1.5 1.5"/>`,
  'database': `<ellipse ${STROKE} cx="12" cy="6" rx="8" ry="3"/><path ${STROKE} d="M4 6v6c0 1.7 3.6 3 8 3s8-1.3 8-3V6M4 12v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6"/>`,
  'line-chart': `<path ${STROKE} d="M3 3v18h18"/><path ${STROKE} d="M7 14l3-4 3 3 4-6"/>`,
  // The bet (screen 1a)
  'close': `<path ${STROKE} d="M18 6 6 18M6 6l12 12"/>`,
  'check': `<path ${STROKE} d="M20 6 9 17l-5-5"/>`,
};

export function icon(name: keyof typeof P | string, size = 20, cls = ''): string {
  return `<svg viewBox="0 0 24 24" width="${size}" height="${size}" class="${cls}" aria-hidden="true">${P[name] ?? ''}</svg>`;
}
