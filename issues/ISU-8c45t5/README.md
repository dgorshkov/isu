---
schema: 1
id: ISU-8c45t5
title: M2-S2 · Load every issue from a ref
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-51kzyr
blocked_by: ISU-b2bwv8
acceptance: loading is correct on all of the above.
---
**Done** #6, 2026-08-26. **The return type below was corrected as this story was built.** It
reads `map[string]Issue`, and it cannot be one: M2-S3 requires a half-written issue to be
reported rather than fatal, and a map of the issues that loaded has nowhere to say which
ones did not. So both loaders return a `Set`, carrying `Issues` and `Broken`. Decoding stays
separate from validating — a bug with no repro is in `Issues`, because `isu check` cannot
report what the loader refused to hand it, and only a file that could not be parsed at all
is in `Broken`. Issues are keyed by **folder name** even where the frontmatter's `id`
disagrees, because that is what every `parent:` and `blocked_by:` in a repository points at
and `Validate` already reports the difference. Two cases the story's list does not name and
the read path forces: two byte-identical issue files are one blob, so the object-id mapping
is one-to-many or one of them is lost; and an unborn HEAD loads as an empty board while a
named ref that does not resolve stays an error, or a typo renders as a repository with no
issues in it. Comments and attachments are not read at a ref — nothing on the board or in a
derivation reads them.
**Branch** `isu/M2-S2-load`
**Build** `repo.LoadRef(ref) map[string]Issue` using the mandated read path: `ls-tree -r` for
object ids, one `cat-file --batch` fed object ids, stream-parse the output.
**Tests first** load from trunk, from a branch, from a detached SHA, from an empty repo;
issues present on one ref and absent on another; a blob containing the batch delimiter
sequence in its body (this will break a naive parser — write that test first).
**Done when** loading is correct on all of the above.
