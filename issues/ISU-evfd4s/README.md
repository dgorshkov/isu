---
schema: 1
id: ISU-evfd4s
title: M8-S3 · Docs, gates and deploy
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-dj6g6k
blocked_by: ISU-m1bcq5
acceptance: every command in the docs actually runs and the site is live from CI.
---
**Done** #21, 2026-09-09. Eight pages under `docs/`, every console block in them run against a
scratch repository during `make test` and `make site`. Two facts about them are worth writing
down rather than discovering:

- **`docs/importing.md` documents a milestone that does not exist yet.** M7 has not been built,
  so the page opens by saying the command is not in this build, publishes the mapping the plan
  specifies, and contains no `$ isu` line at all — there is nothing to run and it does not
  pretend there is. It is the honest version of a page M8-S3 asks for and M7 has not earned.
- **`docs/getting-started.md` runs against a repository it is allowed to write to**, so `isu
  init` and `isu new` actually run. What it cannot assert is anything containing a generated id,
  because an id is thirty bits of hash over eight random bytes; those blocks run and their output
  is not compared, and the page says so where it quotes one.

**`docs/importing.md` is a debt this milestone hands to M7, and the next session should collect
it.** M7 is written and open as #15 against the same base as this pull request; the two were
merged in the order the reviewer chose, this one first, so the moment #15 lands that page's
first paragraph is false and its mapping is documentation of something that ships. What it owes
is small and specific: drop the "not in this build" note, and give the page ```console blocks
running `isu import github` against a recorded dump, so the importer's documentation executes
like every other page here. Merging #15 will also conflict in this file — both pull requests
mark a milestone done in the same table and add `**Done**` paragraphs a few lines apart.

**Netlify was rewriting the pages CI had just verified, and nothing in this repository could
have told anybody.** Pretty URLs post-processing is on by default and is a dashboard form, so
every deploy preview served an `index.html` 46 bytes shorter than the committed one — every
internal href rewritten from `docs/json.html` to `/docs/json`, every attribute requoted from `"`
to `'`. `site.css` and `og.png` came through untouched; only HTML was changed. The third
consequence is the one that matters: every gate runs inside `Build` over the bytes in
`web/site`, and not one of them had ever seen a byte a reader was served — `gateLinks` proved
`docs/json.html` resolves, and the reader got `/docs/json`.

**`skip_processing = true` did not fix it, and finding that out took a deploy.** With that key in
`netlify.toml`, the preview for `32cbc65` still served 13,233 bytes against 13,283 committed and
every href still rewritten. `[build.processing.html] pretty_urls = false` is what the platform
honours: on the preview for `d9a43e6`, `index.html`, `docs/json.html`,
`docs/getting-started.html` and `404.html` are byte for byte what `make site` wrote. Both keys
are kept — the general one is the intent, the specific one is what works — and
`scripts/site_test.go` asserts both are present and nothing more, because a test here cannot
fetch a deploy. So the gates now run over the bytes a reader receives, and **nothing in this
repository holds them to that**: somebody turning post-processing back on in the dashboard would
break it invisibly. Confirming it is a curl and a diff against `web/site`, which is how both the
defect and the fix were established; a check that does it on every deploy needs production to
exist and is a story of its own.

That is the second time this milestone that a gate was believed rather than measured — the first
was the accessibility pass that had never looked at what the stylesheet did to the document it
read. The pattern is worth naming: a gate over an artifact says nothing about the artifact
somebody actually receives.

**The gates are hand-written over the built site, and what they can and cannot see is stated in
`internal/site/gates.go`.** There is no browser in this build, so "no horizontal scroll at
360 px" is enforced as the two things that cause it — a fixed width wider than the viewport, and
wide content outside a box that scrolls — rather than measured. That claim was checked once
against a real Chromium at 360 px while the story was being built, and the page's `scrollWidth`
equalled its `clientWidth`; the gate that runs on every build is the proxy, and it is a proxy on
purpose rather than a browser dependency in the allowlist.

The markdown renderer is `internal/site/markdown.go` and it is not the thing the out-of-scope
list drops. It renders a fixed subset, refuses a line it does not understand, never passes raw
HTML through — a `<script>` in a source document is escaped and rendered as text — and is never
handed an issue body. There is nothing for a sanitiser to do and no configuration in which
there would be.

**Netlify publishes the site, and `.github/workflows/site.yml` publishes nothing.** The story
asks for publishing on merge to trunk from CI and for a workflow lint asserting the deploy job
triggers only on trunk; the site was already wired to Netlify while this milestone was being
built, so the deploy job would have been a second publisher racing the first. What the workflow
does instead is the half that makes the site reviewable: it regenerates web/site on every pull
request and `git diff --exit-code -- web/site` holds the committed bytes to the built ones, and
it runs every console block in the docs while it is there.

The lint changed subject with it, and the replacement is stronger in one direction and weaker in
another — both are worth stating. Stronger: the workflow now holds no write permission at all,
which `scripts/site_test.go` asserts line by line, so nothing it runs on a pull request from
anywhere can reach the address people read. **Weaker: the guarantee that production comes from
trunk left this repository with the deploy job.** It is Netlify's production-branch setting now,
and no test here can see it. `netlify.toml` pins everything that can be pinned in a file — the
publish directory, asserted against the directory `make site` writes, and the absence of a build
command — and the branch is not one of them. A reviewer who wants that guarantee back wants the
Pages job back, and this paragraph is where the trade was made.

`SiteURL` is `https://isu-website.netlify.app`, the one absolute URL on the site.

**One statement in this package is uncovered and is argued for**, in the shape the definition of
done asks for: `documents` propagating a failure from `Renderer.Page`. Every other error return
here is reached — by a source file that is not there, by a stylesheet with no tokens in it, by a
directory something else is sitting on, and by a git on `PATH` that refuses one invocation — but
that one needs `html/template` to fail on a `docBody` this package built out of its own types,
which it cannot. The renderer's own execution failure *is* reached, directly, in
`render_test.go`.

**A valueless attribute hid the one written after it, and that is the hole the gates had.**
Corrected in #15. `openTag` split an attribute list on the first `=` anywhere in what was left of
the tag, so `<script async src="...">` parsed as a single attribute named `async src` and the
element carried no `src` at all. `gateLinks` and `gateThirdParty` both ask an element for its
`src`, so writing `async` in front of one walked a third-party script past every gate here and
the build reported the site clean. The key is now the last word before the `=`, and the words in
front of it are recorded as the barewords they are — which also fixes `<input disabled readonly>`
having been read as one attribute with a space in its name.

**Netlify is injecting markup again, and again nothing in this repository could see it.** The
paragraph above says that somebody turning post-processing back on in the dashboard would break
the site invisibly and that confirming it is a curl and a diff against `web/site`. Doing exactly
that on 2026-09-09 found three insertions on every published page: an HTML comment advertising
Netlify with UTM parameters, `<meta name="hosting-provider">` and `<meta name="netlify-deploy">`,
and `<script async src="/.netlify/scripts/hud?variant=public">` after the closing `</html>`.
That is the **Powered by Netlify badge**, which is on by default for Free-plan projects created
on or after 19 August 2026 and is turned off at Project configuration → General → Powered by
Netlify badge. There is no `netlify.toml` key for it, so this repository cannot pin it and this
paragraph is the record instead. With the parser defect above fixed, the gates run over the
served `index.html` report `/.netlify/scripts/hud?variant=public points at
.netlify/scripts/hud?variant=public, which this build does not produce`; before it, they passed
those bytes clean. Third time, same pattern.

**And the deploy preview is clean, which is the sharpest version of that pattern yet.**
`https://deploy-preview-15--isu-website.netlify.app/docs/importing.html` is byte for byte the
16,110 bytes `make site` wrote — no comment, no meta tags, no script. The badge is a property of
the *public project*, and a preview is not one, so the artifact a reviewer opens on a pull
request and the artifact a reader is served are now provably different documents. Checking the
preview says nothing about production. Only production says anything about production.
**Branch** `isu/M8-S3-docs-deploy`
**Build** `docs/`: getting started, the data model, every derived status with its rule, the
check catalogue, the JSON contract, importing from GitHub Issues, and a page on what isu
deliberately does not do. Then `make site` producing the whole site from a clean checkout, the
gates —
internal link checker, HTML validity, a 300 KB per-page weight budget, an accessibility pass,
responsive down to 360 px, `prefers-reduced-motion` honoured — and publishing on merge to trunk
from CI, with favicon, Open Graph and Twitter cards, sitemap, canonical URLs and a
404 page that is useful rather than decorative.
**Tests first** a test extracting every fenced shell block from the docs and running it against
a scratch repo — documentation that does not execute is documentation that rots, and these docs
will be read by agents; the budget and accessibility tests fail on any violation; a viewport
test asserts no horizontal scroll at 360 px; a workflow-lint test asserts the deploy job
triggers only on trunk; a test asserts every page has a title, a description and an OG image.
**Done when** every command in the docs actually runs and the site is live from CI.

---
