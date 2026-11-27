#!/usr/bin/env bash
# Checks that a resume HTML file fits on one page, and prints it to PDF.
#
#   render.sh <resume.html> [out.pdf]
#
# Prints "FITS slack=<px>" (unused vertical space on the page) or "OVERFLOW by=<px>", then writes
# the PDF (default: resume.pdf next to the HTML). Exits 1 on overflow, without writing the PDF.
# Uses headless Google Chrome. The resume's .page div is a fixed 8.5in x 11in box with
# overflow:hidden, so overflowing content would be silently clipped rather than spill onto page 2;
# this script is how to tell.
set -euo pipefail

src="${1:?usage: render.sh <resume.html> [out.pdf]}"
src="$(cd "$(dirname "$src")" && pwd)/$(basename "$src")"
out="${2:-$(dirname "$src")/resume.pdf}"
chrome="${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# Measure: content height of .page's children vs. the page's inner height.
cat > "$tmp/measure.js" <<'JS'
<script>
window.addEventListener('load', () => {
  const page = document.querySelector('.page');
  const cs = getComputedStyle(page);
  const inner = page.clientHeight - parseFloat(cs.paddingTop) - parseFloat(cs.paddingBottom);
  let used = 0;
  for (const el of page.children) {
    const s = getComputedStyle(el);
    used += el.getBoundingClientRect().height + parseFloat(s.marginTop) + parseFloat(s.marginBottom);
  }
  const r = document.createElement('pre');
  r.id = 'fit-result';
  r.textContent = 'FITRESULT ' + Math.round(inner - used);
  document.body.appendChild(r);
});
</script>
JS

sed "s#</body>#$(tr -d '\n' < "$tmp/measure.js" | sed 's/[&#]/\\&/g')</body>#" "$src" > "$tmp/measure.html"
slack="$("$chrome" --headless --disable-gpu --no-sandbox --virtual-time-budget=2000 --dump-dom "file://$tmp/measure.html" 2>/dev/null \
  | grep -oE 'FITRESULT -?[0-9]+' | awk '{print $2}' | tail -n1)"

if [[ -z "$slack" ]]; then
  echo "ERROR could not measure $src" >&2
  exit 2
fi
if (( slack < 0 )); then
  echo "OVERFLOW by=$(( -slack ))px"
  exit 1
fi
echo "FITS slack=${slack}px"

# Print with no browser margins/headers so the 8.5x11 .page maps onto exactly one sheet.
sed 's#</head>#<style>@page { size: letter; margin: 0; }</style></head>#' "$src" > "$tmp/print.html"
"$chrome" --headless --disable-gpu --no-sandbox --no-pdf-header-footer --print-to-pdf="$out" "file://$tmp/print.html" 2>/dev/null
echo "PDF $out"
