# Slides Creation Validation Checklist

Use this after generating XML and again after creating or editing the deck.

## Before API Execution

- XML is well-formed.
- User text is escaped: `&`, `<`, and `>` are safe.
- Each slide has one clear message.
- Text boxes are sized for expected content.
- Images use `@./local-path` only where `+create --slides` supports it; otherwise they use `file_token`.
- Run execution-layer lint when XML is in a file:

```bash
python3 skills/lark-slides-creator/scripts/layout_lint.py --input /tmp/presentation.xml
```

## After Creation

- Record `xml_presentation_id`.
- Read the full deck with `xml_presentations get`.
- Confirm expected page count and page order.
- Confirm key titles, body text, metrics, and image elements exist.
- Check for blank pages, missing text, truncated shell arguments, unresolved `@` paths, and wrong image `src`.
- Fix localized issues with `+replace-slide`; only delete/recreate a page when the whole structure is wrong.
