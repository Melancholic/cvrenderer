// ATS-friendly CV template — single-column, plain-text-first, so resume parsers
// read it reliably (no sidebars, rating bars or multi-column flow). Data +
// computed variables come from _common.typ; this file is only the layout.
// Fonts come from --font-path (Lato).
#import "@preview/cmarker:0.1.1"
#import "_common.typ": *

// Palette shared with the helsinki template.
#let navy = rgb("#22364e")   // name, section headings & rules
#let ink = rgb("#2b2b2b")    // body text
#let muted = rgb("#9a9a9a")  // meta: locations, issuer · date, tech tags
#let accent = rgb("#1a4971") // links (email, URLs)

// Vertical rhythm, all multiples of `u`:
//   u           single — between lines and heading → body
//   gap-entry   double — between entries
//   gap-section triple — before each section heading (and after the page header)
#let u = 0.45em
#let gap-entry = 2 * u
#let gap-section = 3 * u

// Lists: wrapped lines within an item sit closer than the gap between items.
#let line-gap = 0.35em // between wrapped lines (leading)
#let item-gap = 0.4em // between list items (> line-gap)

// ---- type scale (shared values across both templates) ---------------------
#let fs-name = 24pt    // the person's name
#let fs-h1 = 15pt      // main section headings (and the header subtitle)
#let fs-h2 = 13pt      // entry titles: position / degree / company, project & cert name
#let fs-h3 = 12pt      // secondary headings (sidebar sections in helsinki)
#let fs-regular = 10pt // body text and meta (dates, locations, URLs, tags)

// Markdown body copy for free-form descriptions; • main / ○ nested bullets.
#let md-body(markdown) = {
  set list(marker: ([•], [○], [‣]), spacing: item-gap, body-indent: 0.5em)
  set par(leading: line-gap, spacing: item-gap)
  cmarker.render(subst(markdown), h1-level: 3)
}

#let section(title) = block(above: gap-section, below: u, width: 100%, breakable: false)[
  #text(size: fs-h1, weight: "bold", fill: navy, tracking: 0.5pt)[#upper(title)]
  #v(2.5pt, weak: true)
  #line(length: 100%, stroke: 0.8pt + navy)
]

// The trailing `#" "` is for parsers, not layout: `h(1fr)` emits no character,
// so without it the text layer reads "…Northwind LogisticsMar 2021 - Present".
#let entry-head(titleText, dateText, top: gap-entry) = block(above: top, below: 0em)[
  #text(size: fs-h2, weight: "bold")[#titleText#" "]
  #h(1fr)
  #text(size: fs-h2, weight: "bold")[#dateText]
]

#let date-range(s, e) = if s != none and e != none [#s - #e] else if s != none [#s] else if e != none [#e] else []

// Keep an entry on one page so no heading or lone trailing bullet is stranded.
// The measure is a guard: `breakable: false` on a block taller than the text
// area overflows the page rather than wrapping, so oversized entries stay
// breakable.
#let keep-together(body) = layout(size => {
  let h = measure(block(width: size.width, body)).height
  block(breakable: h > 0.5 * size.height, body)
})

// "https://www.linkedin.com/in/x/" -> "linkedin.com/in/x". The header prints the
// address so a parser that ignores link annotations still reads it.
#let short-url(u) = {
  let s = str(u)
  if s.starts-with("https://") { s = s.slice(8) } else if s.starts-with("http://") { s = s.slice(7) }
  if s.starts-with("www.") { s = s.slice(4) }
  if s.ends-with("/") { s = s.slice(0, s.len() - 1) }
  s
}

// A `label` that just repeats the address would print the same text twice
// across the row, so those fall back to a generic name.
#let link-label(l, short) = {
  let lb = field(l, "label")
  if lb == none or str(lb) == short { "Website" } else { str(lb) }
}

// ---- page ------------------------------------------------------------------
#set document(title: field(data, "name", default: "CV"), author: field(data, "name", default: ""))
#set page(paper: "a4", margin: (x: 15mm, top: 13mm, bottom: 14mm))
#set text(font: "Lato", size: fs-regular, fill: ink)
#set par(leading: 0.4em, justify: false)

// ---- header ----------------------------------------------------------------
#let header-left = {
  block(below: 6pt)[#text(size: fs-name, weight: "bold", fill: navy)[#upper(field(data, "name", default: ""))]]
  let title = field(data, "title")
  if title != none {
    block(below: 7pt)[#text(size: fs-h1, weight: "bold")[#upper(subst(title))]]
  }

  let d = field(data, "details", default: (:))
  let contact = ()
  if field(d, "location") != none { contact.push([#field(d, "location")]) }
  if field(d, "email") != none {
    contact.push(link("mailto:" + field(d, "email"))[#underline(text(fill: accent)[#field(d, "email")])])
  }
  if field(d, "phone") != none { contact.push([#field(d, "phone")]) }

  let pd = ()
  if field(d, "nationality") != none { pd.push(str(field(d, "nationality"))) }
  if field(d, "driving_license") != none { pd.push(str(field(d, "driving_license"))) }
  if field(data, "birth_date") != none { pd.push(str(field(data, "birth_date"))) }
  if field(data, "birth_place") != none { pd.push(str(field(data, "birth_place"))) }

  // The label's trailing `#" "` is the `entry-head` fix again — grid cells are
  // separate text runs, so without it the text reads "LinkedInlinkedin.com/…".
  let profiles = ()
  for l in field(data, "links", default: ()) {
    let url = field(l, "url")
    if url != none {
      let short = short-url(url)
      profiles.push(text(size: fs-regular, weight: "bold")[#link-label(l, short)#" "])
      profiles.push(link(url)[#underline(text(size: fs-regular, fill: accent)[#short])])
    }
  }

  if contact.len() > 0 {
    block(below: 0pt)[#text(size: fs-regular)[#contact.join([ | ])]]
  }
  if pd.len() > 0 {
    set par(leading: 0.55em)
    block(above: 6pt)[
      #text(size: fs-regular)[#pd.join(" | ")]
    ]
  }
  if profiles.len() > 0 {
    block(above: 6pt, below: 0pt)[
      #grid(columns: (auto, auto), column-gutter: 4mm, row-gutter: 2.5pt, ..profiles)
    ]
  }
}

#let photo = field(data, "photo")
#if photo != none {
  grid(
    columns: (1fr, auto), column-gutter: 6mm, align: (left, right),
    header-left,
    box(image(photo, width: 32mm)),
  )
} else {
  header-left
}
#v(6pt)
#line(length: 100%, stroke: 1pt + navy)

// ---- sections ---------------------------------------------------------------

#let summary = field(data, "summary")
#if summary != none {
  section("Professional Summary")
  par(justify: true)[#subst(summary)]
}

// The heading stays the canonical "Technical Skills": parsers match headings
// against a dictionary of known names, and embellishments ("& keywords") miss.
#let techskills = field(data, "technical_skills", default: ())
#if techskills.len() > 0 {
  section("Technical Skills")
  for g in techskills {
    let items = field(g, "items", default: ())
    if items.len() > 0 {
      block(above: u)[#text(weight: "bold")[#field(g, "category", default: "")]: #items.join(", ")]
    }
  }
}

#let experience = field(data, "experience", default: ())
#if experience.len() > 0 {
  section("Employment History")
  for (i, job) in experience.enumerate() {
    let comp = field(job, "company")
    let titleText = if comp != none [#field(job, "position", default: ""), #comp] else [#field(job, "position", default: "")]
    block(above: if i == 0 { u } else { gap-entry }, below: 0em)[#keep-together({
      entry-head(titleText, date-range(field(job, "start"), field(job, "end")), top: 0em)
      let loc = field(job, "location")
      if loc != none { block(above: u, below: 0em)[#text(size: fs-regular, fill: muted)[#loc]] }
      let desc = field(job, "description")
      if desc != none { block(above: u)[#md-body(desc)] }
    })]
  }
}

#let achievements = field(data, "key_achievements", default: ())
#if achievements.len() > 0 {
  section("Key Achievements")
  let items = ()
  for a in achievements {
    if type(a) == dictionary {
      let t = field(a, "title")
      let dd = field(a, "description")
      if t != none and dd != none { items.push([*#subst(t)* — #subst(dd)]) } else if t != none { items.push([*#subst(t)*]) } else if dd != none { items.push(subst(dd)) }
    } else {
      items.push(subst(a))
    }
  }
  if items.len() > 0 { list(..items, marker: [•], spacing: item-gap, body-indent: 0.5em) }
}

#let education = field(data, "education", default: ())
#if education.len() > 0 {
  section("Education")
  for (i, ed) in education.enumerate() {
    let inst = field(ed, "institution")
    let titleText = if inst != none [#field(ed, "degree", default: ""), #inst] else [#field(ed, "degree", default: "")]
    block(above: if i == 0 { u } else { gap-entry }, below: 0em)[#keep-together({
      entry-head(titleText, date-range(field(ed, "start"), field(ed, "end")), top: 0em)
      let loc = field(ed, "location")
      if loc != none { block(above: u, below: 0em)[#text(size: fs-regular, fill: muted)[#loc]] }
      let desc = field(ed, "description")
      if desc != none { block(above: u)[#md-body(desc)] }
    })]
  }
}

#let certs = field(data, "certifications", default: ())
#if certs.len() > 0 {
  section("Certifications")
  for (i, c) in certs.enumerate() {
    block(above: if i == 0 { u } else { gap-entry }, below: 0em)[#text(size: fs-h2, weight: "bold")[#field(c, "name", default: "")]]
    let parts = ()
    if field(c, "issuer") != none { parts.push(str(field(c, "issuer"))) }
    if field(c, "date") != none { parts.push(str(field(c, "date"))) }
    if parts.len() > 0 { block(above: u)[#text(size: fs-regular, fill: muted)[#parts.join(" · ")]] }
    let url = field(c, "url")
    if url != none { block(above: u)[#link(url)[#underline(text(size: fs-regular, fill: accent)[#url])]] }
  }
}

#let projects = field(data, "projects", default: ())
#if projects.len() > 0 {
  section("Projects & Open-source")
  for (i, p) in projects.enumerate() {
    block(above: if i == 0 { u } else { gap-entry }, below: 0em)[
      #text(size: fs-h2, weight: "bold")[#field(p, "name", default: "")]
      #let tech = field(p, "tech")
      #if tech != none [#text(size: fs-regular, fill: muted)[#" · "#tech]]
    ]
    let d = field(p, "description")
    if d != none { block(above: u)[#subst(d)] }
    let url = field(p, "url")
    if url != none { block(above: u)[#link(url)[#underline(text(size: fs-regular, fill: accent)[#url])]] }
  }
}

#let langs = field(data, "languages", default: ())
#if langs.len() > 0 {
  section("Languages")
  block(above: u)[#langs.map(l => {
      let n = field(l, "name", default: "")
      let lv = field(l, "level")
      if lv != none { n + " — " + lv } else { n }
    }).join(", ")]
}

#let hobbies = field(data, "hobbies")
#if hobbies != none {
  section("Interests")
  block(above: u)[#hobbies]
}
