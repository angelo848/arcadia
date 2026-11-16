// gen-icons writes the bundled platform icons (frontend/src/assets/platforms/*.svg) and the master drawing of the
// application icon (scripts/icons/arcadia.svg). The drawings are original: hardware silhouettes, no maker logos
// and no <text> (PLT-009, PLT-010).
//
//	node scripts/gen-icons.mjs
//
// The .ico and .png files are made from the master drawing by scripts/gen-icon.go.
import { mkdirSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '..')
const platformsDir = join(root, 'frontend', 'src', 'assets', 'platforms')
const masterDir = join(root, 'scripts', 'icons')

// cross draws a d-pad centered at (cx, cy): two bars of the given thickness and length.
function cross(cx, cy, thick, len, fill) {
  const h = thick / 2
  const l = len / 2
  return (
    `<rect x="${cx - h}" y="${cy - l}" width="${thick}" height="${len}" rx=".7" fill="${fill}"/>` +
    `<rect x="${cx - l}" y="${cy - h}" width="${len}" height="${thick}" rx=".7" fill="${fill}"/>`
  )
}

// diamond draws four round buttons around (cx, cy); fills go top, right, bottom, left.
function diamond(cx, cy, gap, r, fills) {
  const at = [
    [cx, cy - gap],
    [cx + gap, cy],
    [cx, cy + gap],
    [cx - gap, cy],
  ]
  return at.map(([x, y], i) => `<circle cx="${x}" cy="${y}" r="${r}" fill="${fills[i]}"/>`).join('')
}

// tile wraps a glyph in the rounded plate: diagonal gradient, soft top light, hairline rim and a drop shadow
// under the glyph body.
function tile({ from, to, body, details }) {
  return (
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="64" height="64">` +
    `<defs>` +
    `<linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="${from}"/><stop offset="1" stop-color="${to}"/></linearGradient>` +
    `<linearGradient id="l" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#fff" stop-opacity=".24"/><stop offset=".55" stop-color="#fff" stop-opacity="0"/></linearGradient>` +
    `</defs>` +
    `<rect width="64" height="64" rx="14" fill="url(#g)"/>` +
    `<rect width="64" height="64" rx="14" fill="url(#l)"/>` +
    `<path d="M14 .6h36A13.4 13.4 0 0 1 63.4 14v36A13.4 13.4 0 0 1 50 63.4H14A13.4 13.4 0 0 1 .6 50V14A13.4 13.4 0 0 1 14 .6z" fill="none" stroke="#fff" stroke-opacity=".16" stroke-width="1.2"/>` +
    `<g transform="translate(0 1.6)" fill="#000" stroke="#000" opacity=".22">${body.replace(/fill="[^"]*"/g, '').replace(/stroke="[^"]*"/g, '')}</g>` +
    body +
    details +
    `</svg>\n`
  )
}

const icons = {
  // Desktop monitor with a play mark.
  windows: {
    from: '#3a97f0',
    to: '#0c4a9e',
    body:
      `<rect x="7" y="11" width="50" height="34" rx="4" fill="#f4f8ff"/>` +
      `<path d="M28 45h8l1.5 6h-11z" fill="#cfdcf0"/>` +
      `<rect x="20" y="50" width="24" height="4" rx="2" fill="#f4f8ff"/>`,
    details:
      `<rect x="10.5" y="14.5" width="43" height="27" rx="1.5" fill="#0d3f8a"/>` +
      `<path d="M10.5 16a1.5 1.5 0 0 1 1.5-1.5h40a1.5 1.5 0 0 1 1.5 1.5v6l-43 12z" fill="#fff" opacity=".1"/>` +
      `<path d="M28 21.5v13a1 1 0 0 0 1.5.86l11-6.5a1 1 0 0 0 0-1.72l-11-6.5A1 1 0 0 0 28 21.5z" fill="#7cc4ff"/>`,
  },
  // Flat rectangular controller: d-pad, two pills, two red buttons.
  nes: {
    from: '#c2333b',
    to: '#6a1119',
    body: `<rect x="6" y="19" width="52" height="26" rx="3.5" fill="#ececf0"/>`,
    details:
      `<rect x="9" y="22" width="46" height="20" rx="2" fill="#2b2b31"/>` +
      `<rect x="9" y="22" width="46" height="4" rx="2" fill="#44444c"/>` +
      cross(18.5, 33, 3.8, 11, '#e2e2e8') +
      `<rect x="27" y="33" width="5" height="2.6" rx="1.3" fill="#9a9aa4"/>` +
      `<rect x="34" y="33" width="5" height="2.6" rx="1.3" fill="#9a9aa4"/>` +
      `<circle cx="44" cy="34" r="3.3" fill="#e5383f"/>` +
      `<circle cx="51" cy="34" r="3.3" fill="#e5383f"/>`,
  },
  // Dog-bone controller with the four coloured buttons.
  snes: {
    from: '#9a93bd',
    to: '#565078',
    body:
      `<circle cx="19" cy="32" r="13" fill="#eeeef3"/>` +
      `<circle cx="45" cy="32" r="13" fill="#eeeef3"/>` +
      `<rect x="19" y="21.5" width="26" height="19" fill="#eeeef3"/>`,
    details:
      `<circle cx="19" cy="32" r="8.6" fill="#dcdce4"/>` +
      `<circle cx="45" cy="32" r="8.6" fill="#dcdce4"/>` +
      cross(19, 32, 4, 12.5, '#3b3b46') +
      `<path d="M28.6 35.2l2.6-2.4M33 35.2l2.6-2.4" stroke="#8b8b98" stroke-width="2.1" stroke-linecap="round"/>` +
      diamond(45, 32, 4.9, 2.6, ['#3d74e6', '#e23d42', '#f0c132', '#2fae5e']),
  },
  // Three-pronged controller with the centre stick.
  n64: {
    from: '#35a863',
    to: '#125a35',
    body:
      `<rect x="7" y="16" width="50" height="17" rx="8.5" fill="#e6e7ec"/>` +
      `<rect x="8.5" y="22" width="11.5" height="27" rx="5.75" transform="rotate(13 14 24)" fill="#e6e7ec"/>` +
      `<rect x="26.25" y="22" width="11.5" height="31" rx="5.75" fill="#e6e7ec"/>` +
      `<rect x="44" y="22" width="11.5" height="27" rx="5.75" transform="rotate(-13 50 24)" fill="#e6e7ec"/>`,
    details:
      `<path d="M26.25 33v14.25a5.75 5.75 0 0 0 11.5 0V33z" fill="#d3d5dc"/>` +
      cross(15.5, 24.5, 3, 8.5, '#3a3d45') +
      `<circle cx="32" cy="22" r="1.5" fill="#e23d42"/>` +
      `<circle cx="32" cy="31" r="4" fill="#3a3d45"/>` +
      `<circle cx="32" cy="31" r="2" fill="#9aa0ab"/>` +
      `<circle cx="42.5" cy="23.2" r="2.1" fill="#2fae5e"/>` +
      `<circle cx="45.2" cy="28" r="2.1" fill="#3d74e6"/>` +
      `<circle cx="49.5" cy="22" r="1.5" fill="#f0c132"/>` +
      `<circle cx="52" cy="25.8" r="1.5" fill="#f0c132"/>`,
  },
  // The cube, seen from a corner, with the round lid.
  gamecube: {
    from: '#5f70ec',
    to: '#232a8f',
    body: `<path d="M32 11.5L50.5 22v21L32 53.5 13.5 43V22z" fill="#f4f2ff" stroke="#f4f2ff" stroke-width="2.4" stroke-linejoin="round"/>`,
    details:
      `<path d="M13.5 22L32 32.5v21L13.5 43z" fill="#cbc4f3"/>` +
      `<path d="M50.5 22L32 32.5v21L50.5 43z" fill="#a297e6"/>` +
      `<ellipse cx="32" cy="22" rx="9.5" ry="5.2" fill="none" stroke="#5f70ec" stroke-width="1.7"/>` +
      `<ellipse cx="32" cy="22" rx="3" ry="1.6" fill="#5f70ec"/>` +
      `<path d="M17.5 34.6l4.2 2.4M24.2 38.4l4.2 2.4" stroke="#232a8f" stroke-width="2.2" stroke-linecap="round"/>` +
      `<circle cx="45.5" cy="38.5" r="1.5" fill="#f4f2ff"/>`,
  },
  // The tilted remote.
  wii: {
    from: '#4cc6f0',
    to: '#1379b3',
    body: `<rect x="23.5" y="7" width="17" height="50" rx="5.5" transform="rotate(18 32 32)" fill="#f7f9fb"/>`,
    details:
      `<g transform="rotate(18 32 32)">` +
      `<circle cx="27.6" cy="11.8" r="1.3" fill="#e5484d"/>` +
      cross(32, 18.5, 3, 9, '#aeb9c4') +
      `<circle cx="32" cy="29.5" r="4.2" fill="#e6ebf0" stroke="#aeb9c4" stroke-width="1.2"/>` +
      `<circle cx="27.5" cy="38" r="1.5" fill="#aeb9c4"/>` +
      `<circle cx="32" cy="38" r="1.5" fill="#4cc6f0"/>` +
      `<circle cx="36.5" cy="38" r="1.5" fill="#aeb9c4"/>` +
      `<circle cx="32" cy="45" r="2.3" fill="#e6ebf0" stroke="#aeb9c4" stroke-width="1.1"/>` +
      `<circle cx="32" cy="51" r="2.3" fill="#e6ebf0" stroke="#aeb9c4" stroke-width="1.1"/>` +
      `</g>`,
  },
  // Tablet between two detachable controllers.
  switch: {
    from: '#ff5261',
    to: '#b80f2a',
    body:
      `<path d="M14.5 19H19v26h-4.5A7.5 7.5 0 0 1 7 37.5v-11A7.5 7.5 0 0 1 14.5 19z" fill="#fff"/>` +
      `<rect x="19" y="19" width="26" height="26" fill="#25272d"/>` +
      `<path d="M45 19h4.5a7.5 7.5 0 0 1 7.5 7.5v11A7.5 7.5 0 0 1 49.5 45H45z" fill="#fff"/>`,
    details:
      `<rect x="21" y="21.5" width="22" height="21" rx="1" fill="#454b5a"/>` +
      `<path d="M21 22.500a1 1 0 0 1 1-1h21v5.500L21 37z" fill="#fff" opacity=".12"/>` +
      `<circle cx="13" cy="27" r="2.5" fill="#25272d"/>` +
      diamond(13, 37, 2.2, 0.95, ['#25272d', '#25272d', '#25272d', '#25272d']) +
      diamond(51, 27, 2.2, 0.95, ['#25272d', '#25272d', '#25272d', '#25272d']) +
      `<circle cx="51" cy="37" r="2.5" fill="#25272d"/>`,
  },
  // The larger tablet, with the coloured rings under the sticks.
  switch2: {
    from: '#565a66',
    to: '#1d1f25',
    body:
      `<path d="M12 17.500H16v29h-4A8 8 0 0 1 4 38.500v-13A8 8 0 0 1 12 17.500z" fill="#eceef2"/>` +
      `<rect x="16" y="17.5" width="32" height="29" fill="#0f1014"/>` +
      `<path d="M48 17.500h4a8 8 0 0 1 8 8v13a8 8 0 0 1-8 8h-4z" fill="#eceef2"/>`,
    details:
      `<rect x="14.2" y="17.5" width="1.8" height="29" fill="#36b7ff"/>` +
      `<rect x="48" y="17.5" width="1.8" height="29" fill="#ff5a4f"/>` +
      `<rect x="18" y="19.5" width="28" height="25" rx="1" fill="#39415c"/>` +
      `<path d="M18 20.500a1 1 0 0 1 1-1h27v6L18 39z" fill="#fff" opacity=".12"/>` +
      `<circle cx="9.6" cy="26" r="3" fill="#36b7ff"/>` +
      `<circle cx="9.6" cy="26" r="1.9" fill="#1d1f25"/>` +
      diamond(9.6, 37.5, 2.1, 0.9, ['#1d1f25', '#1d1f25', '#1d1f25', '#1d1f25']) +
      diamond(54.4, 26, 2.1, 0.9, ['#1d1f25', '#1d1f25', '#1d1f25', '#1d1f25']) +
      `<circle cx="54.4" cy="37.5" r="3" fill="#ff5a4f"/>` +
      `<circle cx="54.4" cy="37.5" r="1.9" fill="#1d1f25"/>`,
  },
  // Landscape handheld with the screen in the middle.
  gba: {
    from: '#9a5fe0',
    to: '#471f8c',
    body: `<path d="M17 19.500h30c7 0 12 4.5 12 11.500s-5 13.5-12 13.500H17c-7 0-12-6.5-12-13.500s5-11.5 12-11.500z" fill="#eeecfa"/>`,
    details:
      `<rect x="20" y="22.5" width="24" height="18" rx="2.2" fill="#2a2544"/>` +
      `<rect x="22.5" y="25" width="19" height="13" rx=".8" fill="#9fd9c3"/>` +
      `<path d="M22.5 25.800a.8.8 0 0 1 .8-.8h18.200v3.500L22.5 34z" fill="#fff" opacity=".28"/>` +
      cross(12.5, 30.5, 2.9, 8.5, '#4a4174') +
      `<circle cx="52.5" cy="28.8" r="2.3" fill="#4a4174"/>` +
      `<circle cx="48.2" cy="33" r="2.3" fill="#4a4174"/>` +
      `<path d="M49.5 39.500l3.5-2.500M51.5 41.200l3.3-2.4" stroke="#b7b1de" stroke-width="1.1" stroke-linecap="round"/>` +
      `<circle cx="11.3" cy="38.5" r=".9" fill="#b7b1de"/>` +
      `<circle cx="14.3" cy="38.5" r=".9" fill="#b7b1de"/>`,
  },
  // Open clamshell with the two screens.
  '3ds': {
    from: '#27bccd',
    to: '#0c6d80',
    body:
      `<rect x="11" y="6.5" width="42" height="25" rx="4.5" fill="#f4f6f8"/>` +
      `<rect x="13.5" y="29.5" width="37" height="4" fill="#c5ced6"/>` +
      `<rect x="11" y="32.5" width="42" height="25" rx="4.5" fill="#f4f6f8"/>`,
    details:
      `<rect x="15.5" y="10" width="33" height="17.5" rx="1.2" fill="#1e2932"/>` +
      `<path d="M15.5 11.200a1.2 1.2 0 0 1 1.2-1.200h31.800v4L15.5 22z" fill="#fff" opacity=".13"/>` +
      `<rect x="22" y="36" width="20" height="14.5" rx="1.2" fill="#1e2932"/>` +
      `<circle cx="16.5" cy="40" r="2.7" fill="#97a3ad"/>` +
      cross(16.5, 48.5, 2.2, 6.4, '#97a3ad') +
      diamond(47.5, 43.5, 2.5, 1.1, ['#97a3ad', '#97a3ad', '#97a3ad', '#97a3ad']) +
      `<circle cx="29" cy="53.8" r=".8" fill="#c5ced6"/>` +
      `<circle cx="32" cy="53.8" r=".8" fill="#27bccd"/>` +
      `<circle cx="35" cy="53.8" r=".8" fill="#c5ced6"/>`,
  },
  // Console seen from above: the round lid and the two big buttons.
  ps1: {
    from: '#a3acbe',
    to: '#565f73',
    body: `<rect x="7" y="14" width="50" height="36" rx="3.5" fill="#eceef1"/>`,
    details:
      `<rect x="7" y="43" width="50" height="7" rx="3.5" fill="#d5d8de"/>` +
      `<rect x="7" y="43" width="50" height="3" fill="#d5d8de"/>` +
      `<circle cx="32" cy="27.5" r="11.5" fill="#dfe2e7" stroke="#b4b9c3" stroke-width="1.2"/>` +
      `<circle cx="32" cy="27.5" r="4" fill="none" stroke="#b4b9c3" stroke-width="1.2"/>` +
      `<path d="M24.5 21.500a9.5 9.5 0 0 1 9-3.4" fill="none" stroke="#fff" stroke-width="1.4" stroke-linecap="round"/>` +
      `<circle cx="13.5" cy="37" r="3" fill="#c9cdd5" stroke="#aab0bb" stroke-width="1"/>` +
      `<circle cx="50.5" cy="37" r="3" fill="#c9cdd5" stroke="#aab0bb" stroke-width="1"/>` +
      `<circle cx="13.5" cy="20" r="1.1" fill="#2fae5e"/>` +
      `<rect x="21.5" y="45.3" width="8" height="2.3" rx=".6" fill="#7f8796"/>` +
      `<rect x="34.5" y="45.3" width="8" height="2.3" rx=".6" fill="#7f8796"/>`,
  },
  // The black console lying flat, seen from the front: tray on top, ports and ribbed grille below.
  ps2: {
    from: '#5f90ff',
    to: '#2440bd',
    body:
      `<rect x="5" y="19" width="54" height="14" rx="2.2" fill="#20232c"/>` +
      `<rect x="8" y="31" width="48" height="14" rx="2.2" fill="#2c303c"/>`,
    details:
      `<path d="M7.2 19h49.6a2.2 2.2 0 0 1 2.2 2.2v.6H5v-.6A2.2 2.2 0 0 1 7.2 19z" fill="#4a5064"/>` +
      `<rect x="5" y="31.6" width="54" height="1.4" fill="#0e0f14"/>` +
      `<rect x="9" y="24" width="27" height="4.4" rx="1.2" fill="#0c0d11" stroke="#565d74" stroke-width=".9"/>` +
      `<rect x="39.5" y="23.7" width="5" height="5" rx="1.1" fill="#3f8bff"/>` +
      `<circle cx="49.5" cy="26.2" r="1.4" fill="#2fd27a"/>` +
      `<circle cx="54" cy="26.2" r="1.4" fill="#ff5d5d"/>` +
      `<rect x="11" y="35" width="8.5" height="1.8" rx=".5" fill="#0c0d11"/>` +
      `<rect x="21.5" y="35" width="8.5" height="1.8" rx=".5" fill="#0c0d11"/>` +
      `<rect x="11" y="38.3" width="8.5" height="4.2" rx="2.1" fill="#0c0d11" stroke="#565d74" stroke-width=".8"/>` +
      `<rect x="21.5" y="38.3" width="8.5" height="4.2" rx="2.1" fill="#0c0d11" stroke="#565d74" stroke-width=".8"/>` +
      `<rect x="33.5" y="39.2" width="3.4" height="2.2" rx=".4" fill="#0c0d11"/>` +
      `<rect x="38.4" y="39.2" width="3.4" height="2.2" rx=".4" fill="#0c0d11"/>` +
      `<path d="M45 35.5v7M47.6 35.5v7M50.2 35.5v7M52.8 35.5v7" stroke="#565d74" stroke-width="1.3" stroke-linecap="round"/>`,
  },
  // Twin-stick gamepad.
  ps3: {
    from: '#2a3446',
    to: '#07090e',
    body:
      `<circle cx="16.5" cy="27.5" r="10" fill="#eef0f5"/>` +
      `<circle cx="47.5" cy="27.5" r="10" fill="#eef0f5"/>` +
      `<rect x="16.5" y="19" width="31" height="19" rx="3" fill="#eef0f5"/>` +
      `<rect x="8.5" y="27" width="12" height="24" rx="6" transform="rotate(15 14.5 30)" fill="#eef0f5"/>` +
      `<rect x="43.5" y="27" width="12" height="24" rx="6" transform="rotate(-15 49.5 30)" fill="#eef0f5"/>`,
    details:
      cross(16.5, 27, 3.2, 9.5, '#2a3446') +
      diamond(47.5, 27, 3.7, 1.6, ['#3ecf8e', '#ff5d5d', '#5aa2ff', '#f08ad0']) +
      `<circle cx="25.5" cy="36" r="4.3" fill="#2a3446"/>` +
      `<circle cx="25.5" cy="36" r="2.2" fill="#8b93a5"/>` +
      `<circle cx="38.5" cy="36" r="4.3" fill="#2a3446"/>` +
      `<circle cx="38.5" cy="36" r="2.2" fill="#8b93a5"/>` +
      `<rect x="28" y="24.5" width="3" height="1.8" rx=".9" fill="#aab1c0"/>` +
      `<rect x="33" y="24.5" width="3" height="1.8" rx=".9" fill="#aab1c0"/>`,
  },
  // Arcade stick: any platform without its own drawing.
  generic: {
    from: '#677186',
    to: '#2b313d',
    body:
      `<circle cx="32" cy="17.5" r="9" fill="#ff5a5f"/>` +
      `<rect x="29.8" y="24" width="4.4" height="20" fill="#c9ced9"/>` +
      `<path d="M25.5 44l2.5-5.500h8l2.5 5.500z" fill="#dde1e9"/>` +
      `<rect x="11" y="43.5" width="42" height="11" rx="3.5" fill="#eef0f5"/>`,
    details:
      `<circle cx="32" cy="17.5" r="9" fill="#ff5a5f"/>` +
      `<circle cx="28.8" cy="14.3" r="2.6" fill="#fff" opacity=".6"/>` +
      `<rect x="11" y="50.5" width="42" height="4" rx="2" fill="#d3d7e0"/>` +
      `<circle cx="18.5" cy="48" r="2" fill="#f0c132"/>` +
      `<circle cx="45.5" cy="48" r="2" fill="#3d74e6"/>`,
  },
}

// The application mark: an "A" whose apex is the ball of an arcade stick.
const appIcon =
  `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="1024" height="1024">` +
  `<defs>` +
  `<linearGradient id="bg" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#4a8dff"/><stop offset=".5" stop-color="#6541f2"/><stop offset="1" stop-color="#2c1489"/></linearGradient>` +
  `<linearGradient id="hi" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#fff" stop-opacity=".28"/><stop offset=".5" stop-color="#fff" stop-opacity="0"/></linearGradient>` +
  `<radialGradient id="ball" cx=".36" cy=".3" r=".8"><stop offset="0" stop-color="#ffe680"/><stop offset=".55" stop-color="#ffb62e"/><stop offset="1" stop-color="#f07a12"/></radialGradient>` +
  `<linearGradient id="leg" gradientUnits="userSpaceOnUse" x1="0" y1="23" x2="0" y2="56"><stop offset="0" stop-color="#fff"/><stop offset="1" stop-color="#dcd6ff"/></linearGradient>` +
  `</defs>` +
  `<rect width="64" height="64" rx="14" fill="url(#bg)"/>` +
  `<rect width="64" height="64" rx="14" fill="url(#hi)"/>` +
  `<path d="M14 .6h36A13.4 13.4 0 0 1 63.4 14v36A13.4 13.4 0 0 1 50 63.4H14A13.4 13.4 0 0 1 .6 50V14A13.4 13.4 0 0 1 14 .6z" fill="none" stroke="#fff" stroke-opacity=".18" stroke-width="1.2"/>` +
  `<g transform="translate(0 1.8)" opacity=".28" fill="none" stroke="#12063f" stroke-linecap="round" stroke-linejoin="round">` +
  `<path d="M17.5 51.500L32 23l14.5 28.5" stroke-width="8.5"/><path d="M24.5 42h15" stroke-width="6.5"/></g>` +
  `<g fill="none" stroke="url(#leg)" stroke-linecap="round" stroke-linejoin="round">` +
  `<path d="M17.5 51.500L32 23l14.5 28.5" stroke-width="8.5"/><path d="M24.5 42h15" stroke-width="6.5"/></g>` +
  `<circle cx="32" cy="19.3" r="10.8" fill="#12063f" opacity=".3"/>` +
  `<circle cx="32" cy="17.5" r="10.3" fill="url(#ball)"/>` +
  `<ellipse cx="28.3" cy="13.5" rx="3.4" ry="2.4" transform="rotate(-30 28.3 13.5)" fill="#fff" opacity=".7"/>` +
  `</svg>\n`

mkdirSync(platformsDir, { recursive: true })
mkdirSync(masterDir, { recursive: true })
for (const [key, icon] of Object.entries(icons)) {
  writeFileSync(join(platformsDir, `${key}.svg`), tile(icon))
}
writeFileSync(join(masterDir, 'arcadia.svg'), appIcon)
