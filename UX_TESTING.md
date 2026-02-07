# Usability Testing Report — pubmed-cli v0.5.1

**Date:** 2026-02-07  
**Tester:** Simulated personas  
**Version:** 0.5.1

---

## Executive Summary

Tested with 4 simulated user personas. Overall the tool is **functional and produces high-quality output**, but has **discoverability issues** for new users and **long wait times** during relevance scoring that could frustrate users.

### Key Findings

| Issue | Severity | Recommendation |
|-------|----------|----------------|
| No progress indicator during scoring | High | Add progress bar or status updates |
| Common typos not handled ("synthesis" vs "synth") | Medium | Add command aliases |
| First-time users may not know which command to use | Medium | Make `wizard` the default or add tutorial |
| ~2 min wait with no feedback feels broken | High | Show "Scoring paper 5/30..." progress |
| API key setup not obvious | Medium | Better onboarding / error messages |

---

## Persona 1: Dr. Sarah Chen — Clinical Researcher

**Profile:** Physician-scientist, writing R01 grant, needs quick lit synthesis  
**Tech comfort:** Moderate (uses R, familiar with terminal)  
**Goal:** Get a citable paragraph on "ketamine for treatment-resistant depression"

### Session

```bash
$ pubmed synth "ketamine for treatment-resistant depression efficacy" \
    --papers 5 --words 300 --docx ketamine.docx --ris ketamine.ris
```

**Wait time:** ~90 seconds (felt long with no progress indicator)

### Outcome
✅ **Success** — Got Word doc + RIS file  
✅ Good quality synthesis with 5 relevant papers  
✅ RIS imported into Zotero without issues

### Pain Points
- ❌ **No progress feedback** — Stared at blank terminal for 90 seconds wondering if it was working
- ❌ Had to look up `--ris` flag (thought it might be `--references` or `--bib`)
- ⚠️ Would have preferred BibTeX over RIS (EndNote user minority)

### Suggestions
> "Would be nice to see 'Searching... Scoring papers... Synthesizing...' so I know it's working."

---

## Persona 2: Marcus — Medical Student

**Profile:** MS3 doing a case report, needs to find papers on a rare condition  
**Tech comfort:** Low (primarily uses Google/PubMed web interface)  
**Goal:** Quick literature summary for "Kawasaki disease cardiac complications"

### Session

```bash
$ pubmed "Kawasaki disease cardiac"
Error: unknown command "Kawasaki disease cardiac" for "pubmed"

$ pubmed synthesis "Kawasaki disease"
Error: unknown command "synthesis" for "pubmed"

$ pubmed --help
[sees list of commands]

$ pubmed synth "Kawasaki disease cardiac complications"
Error: provide a question or use --pmid for single paper
[confused — he did provide a question]

$ pubmed synth "Kawasaki disease cardiac complications" --papers 3
[works after setting LLM env vars]
```

### Outcome
⚠️ **Partial success** — Eventually got output after trial and error

### Pain Points
- ❌ **Tried to just type the question** — expected natural language interface
- ❌ **"synthesis" vs "synth"** — common typo, no alias
- ❌ **No guidance on LLM setup** — had to Google the error message
- ❌ **Quoting confusion** — wasn't sure if quotes were needed

### Suggestions
> "Make it work like I'm just asking a question. `pubmed 'What causes X?'` should just work."

---

## Persona 3: Dr. Raj Patel — Bioinformatician

**Profile:** Building automated literature review pipeline  
**Tech comfort:** High (Python/R/bash daily)  
**Goal:** Pipe JSON output into downstream analysis

### Session

```bash
$ pubmed synth "CRISPR gene therapy sickle cell" --papers 3 --json
[waits ~80 seconds]
{
  "question": "CRISPR gene therapy sickle cell",
  "synthesis": "CRISPR gene therapy holds significant promise...",
  "papers_searched": 30,
  "papers_scored": 30,
  "papers_used": 3,
  "references": [...],
  "ris": "...",
  "tokens": {"input": 9151, "output": 396, "total": 9547}
}
```

### Outcome
✅ **Success** — Clean JSON, easy to parse

### Pain Points
- ⚠️ JSON includes RIS as embedded string (would prefer separate or omit)
- ⚠️ No way to batch multiple queries efficiently
- ✅ Token counts included — very useful for cost tracking

### Suggestions
> "Add `--no-ris` to skip RIS in JSON output. Consider a batch mode for multiple queries."

---

## Persona 4: Emily — First-Time CLI User

**Profile:** Undergrad research assistant, never used terminal before  
**Tech comfort:** Very low (macOS, uses GUI apps)  
**Goal:** "My PI said to use this tool to find papers"

### Session

```bash
$ pubmed
[sees wall of help text]

$ pubmed wizard
[interactive form appears — much better!]
🔬 PubMed Literature Synthesis
What's your research question?
> autism early intervention

[clicks through defaults]
[waits... no spinner visible in some terminals]
[success!]
```

### Outcome
✅ **Success with wizard** — Guided experience worked well

### Pain Points
- ❌ **Help text is overwhelming** — 10 commands, many flags
- ✅ **Wizard saved the day** — step-by-step was perfect for her
- ⚠️ **Didn't know about wizard** — only found it by reading help carefully
- ❌ **No Windows instructions** — PI has Mac, she has Windows

### Suggestions
> "Make wizard the first thing people see. Maybe `pubmed` with no args should launch wizard?"

---

## Quantitative Summary

| Metric | Value |
|--------|-------|
| Task success rate | 4/4 (100%) |
| Time to first success | 30s - 5min |
| Commands tried before success | 1-4 |
| Help text consultations | 3/4 users |
| Would recommend | 3/4 |

---

## Prioritized Recommendations

### P0 — Critical (fix before promotion)

1. **Add progress indicator during synthesis**
   - Show "Scoring paper 5/30..." or a spinner with status
   - 90 seconds of silence feels broken

2. **Better first-run experience**
   - Running `pubmed` with no args could prompt: "Run `pubmed wizard` for guided setup"
   - Or launch wizard directly

### P1 — High (next release)

3. **Add command aliases**
   - `synthesis` → `synth`
   - `summarize` → `synth`
   - `review` → `synth`

4. **Improve error messages for missing LLM config**
   - Current: Raw API error from OpenAI
   - Better: "No LLM configured. Set LLM_API_KEY or use --claude. Run `pubmed config set` to configure."

5. **Add `--bibtex` output option**
   - Many users prefer BibTeX over RIS
   - Or at minimum, document how to convert

### P2 — Medium (future)

6. **Batch mode for multiple queries**
   - `pubmed synth --batch queries.txt --output-dir ./results`

7. **Windows documentation/testing**
   - Ensure ANSI escapes work or degrade gracefully
   - Test wizard on Windows Terminal

8. **JSON output refinements**
   - `--no-ris` to exclude embedded RIS
   - `--no-abstract` to reduce payload size

---

## Appendix: Test Artifacts

- `/tmp/sarah_ketamine.docx` — 12KB Word document
- `/tmp/sarah_ketamine.ris` — 12KB RIS file (5 references)
- JSON output for CRISPR query — valid, parseable

---

*Report generated by automated persona simulation testing*
