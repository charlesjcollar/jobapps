---
name: tailor-resume
description: Build a one-page resume tailored to a job posting, using only points from the master resume, and save it in the posting's archive folder (db/<company_name>/<posting_id>/).
argument-hint: <job posting URL>
disable-model-invocation: true
---

# Tailor resume

Build the best possible one-page resume for the posting at `$ARGUMENTS`, drawing **only** on the
master resume, and save it to `db/<company_name>/<posting_id>/`.

The master resume is `Charlie Collar Resume.html` at the repo root. It and everything under `db/` are
gitignored personal documents: never commit them, never quote their contents into tracked files, and
never send them to any outside service.

## The one rule that matters most

**Nothing may be falsified or exaggerated.** Every claim in the new resume must trace back to a point
in the master resume. In particular:

- No new technologies, tools, domains, metrics, team sizes, or outcomes. If the master doesn't say
  it, the resume doesn't say it, however well it would match the posting.
- Numbers are copied exactly (`85%`, `8x`, `1 million transfers`). Never round up, combine into a
  bigger number, or attach a metric to a different accomplishment.
- Never strengthen ownership or scope: "contributed to" doesn't become "led", "maintained" doesn't
  become "architected", "participated in" doesn't become "drove".
- Never move a point to a different job or role than the one it belongs to in the master.
- Reword for the posting's vocabulary only when it means the same thing (e.g. "REST microservices" →
  "REST APIs" is fine; "message-driven" → "streaming" is not, unless the master supports it).
- When unsure whether a rewording overstates something, use the master's wording.

## Steps

### 1. Read the posting

Fetch `$ARGUMENTS` (WebFetch, or the `web-fetch` agent). Many job boards render with JavaScript and
return an empty shell; if so, try the board's public JSON API for that posting (Greenhouse, Lever,
Ashby, Workday, etc. — the matching plugin in `bff/plugins/` or `bff/plugins/shared/<system>/` shows
which endpoint that company uses). If neither works, ask me to paste the posting text. Fetch once;
don't hammer the site.

Pull out: job title, company, posting ID, responsibilities, required qualifications, preferred
qualifications, and the tech stack named.

### 2. Find the folder

The folder is `db/<company_name>/<posting_id>/`, using the same values as the posting's `jobs` row.
Look the row up by link first (from the repo root):

```sh
echo "SELECT company_name, posting_id, job_title FROM jobs WHERE link = :'link';" \
  | docker compose exec -T db psql -U jobapps -d jobapps -At -F '|' -v link="$ARGUMENTS"
```

If there's no row (or the database isn't running), work it out instead: the company name as that
company's plugin writes it (its `companyName` constant in `bff/plugins/<company>/`), or as the posting
writes it if there's no plugin; and the posting's own ID from the URL or page. Say in the final report
which way the folder was chosen. Replace any `/` in either value with `-`. Create the folder if it
doesn't exist.

### 3. Read the whole master resume

Read the full file, **including HTML comments**. Commented-out `<li>` bullets and the bullet bank in
the trailing comment after `</html>` are all real points, and they're the main pool to draw from. The
bank often has several wordings of the same point; use whichever fits best (they are the same fact,
don't count them twice).

Also note my voice from the visible bullets: how sentences open, how long they run, how impact and
metrics are stated, which words I favor and avoid. Write everything new in that voice — it should read
as if I wrote it, not like generic resume copy. No buzzwords the master doesn't use.

### 4. Map the posting to the master, and check for gaps

For each responsibility and required qualification, find the master points that support it.

Then list the **critical gaps**: core responsibilities or required qualifications (including required
technologies or years in an area) that nothing in the master supports. Leave out nice-to-haves and
anything covered reasonably by an existing point.

If there are critical gaps, **stop and ask me before writing the resume.** For each gap, quote what
the posting asks for and say what (if anything) in the master comes closest. I'll either add a point
to the master resume (re-read it before continuing) or tell you to leave it out. If there are no
critical gaps, say so and carry on.

### 5. Write the resume

Start from a copy of the master and change **only**:

- **Summary** — rewrite it for this posting, from facts in the master (years of experience exactly as
  the master states it, domains and work actually in the bullets). Keep it about as short as the
  master's.
- **Experience bullets** (the `<li>`s inside `.responsibilities`) — choose, reorder, reword, combine,
  or split points to fit the posting. Most relevant first within each role. Every role keeps at least
  one bullet. Don't touch company names, teams, job titles, or dates.
- **Skills** — reorder items within a row, and the rows themselves, to put what the posting asks for
  first. Don't add, remove, rename, or move an item to a different row.

Leave everything else exactly as is: header, contact details, Education, the CSS. In the output, drop
all HTML comments (commented-out bullets and the bullet bank) so the file holds only what's shown.

Save as `db/<company_name>/<posting_id>/resume.html`.

### 6. Fit it on one page

The `.page` box is a fixed letter-size page with `overflow: hidden`, so overflow gets silently cut off
instead of spilling onto a second page. Check with the script next to this file (from the repo root):

```sh
.claude/skills/tailor-resume/render.sh "db/<company_name>/<posting_id>/resume.html"
```

It prints `OVERFLOW by=<px>` (exit 1, no PDF) or `FITS slack=<px>` and writes `resume.pdf` next to the
HTML. On overflow, cut or tighten the least relevant bullet and rerun. Aim to fill the page: if slack is
large (more than ~40px, about two lines), add back the next most relevant point. Bullets that wrap by
only a word or two are cheap to tighten.

### 7. Report

Tell me:

- the folder and how it was chosen (database row or derived), and the paths to `resume.html` and
  `resume.pdf`
- the final fit (slack)
- **a source map**: each bullet in the new resume, with the master point(s) it came from, so I can
  check nothing was invented
- relevant master points left out for space
- the gaps from step 4, if I chose to leave them out
