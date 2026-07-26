// Typst CV template — a two-column layout: a white main column with a
// full-height dark sidebar on the right. Data + computed variables come from
// _common.typ; this file is only the Helsinki layout.
//
// Fonts come from --font-path (Lato). See internal/typst for how the service
// invokes this.
#import "@preview/cmarker:0.1.1"
#import "_common.typ": *

// ---- palette --------------------------------------------------------------
#let navy = rgb("#22364e")
#let ink = rgb("#2b2b2b")
#let muted = rgb("#9a9a9a")
#let side-label = rgb("#8ea0b5")
#let side-text = rgb("#ffffff")
#let bar-track = rgb("#4b5b73")
#let accent = rgb("#1a4971") // link colour in the main column

#let sidebar-w = 68mm

// Vertical rhythm, all multiples of `u` (same scale as primeats):
//   u           single — between lines and heading → body
//   gap-entry   double — between entries
//   gap-section triple — above section headings (main & sidebar)
#let u = 0.45em
#let gap-entry = 2 * u
#let gap-section = 3 * u

// Lists: wrapped lines within an item sit closer than the gap between items.
#let line-gap = 0.35em // between wrapped lines (leading)
#let item-gap = 0.4em // between list items (> line-gap)

// ---- type scale (shared values across both templates) ---------------------
#let fs-name = 24pt    // the person's name
#let fs-h1 = 15pt      // main section headings
#let fs-h2 = 13pt      // entry titles: position / degree / company, project & cert name
#let fs-h3 = 12pt      // secondary headings (sidebar sections)
#let fs-regular = 10pt // body text and meta (dates, locations, URLs, sidebar)

// Render a Markdown string (after {{var}} substitution) as body copy. Used for
// free-form entry descriptions (jobs, education); nesting uses •, ○, ‣ by depth.
#let md-body(markdown) = {
  set list(marker: ([•], [○], [‣]), spacing: item-gap, body-indent: 0.4em)
  set par(leading: line-gap, spacing: item-gap)
  cmarker.render(subst(markdown), h1-level: 3)
}

// ---- page: paint the sidebar full-height on every page --------------------
#set document(title: field(data, "name", default: "CV"), author: field(data, "name", default: ""))
#set page(
  paper: "a4",
  margin: (rest: 0pt),
  background: place(right + top, rect(width: sidebar-w, height: 100%, fill: navy)),
)
#set text(font: "Lato", size: fs-regular, fill: ink)
#set par(leading: 0.4em, justify: false)

// ---- small helpers --------------------------------------------------------
#let initials(name) = {
  let parts = name.split(" ").filter(p => p != "")
  let g(i) = if parts.len() > i { parts.at(i).clusters().at(0, default: "") } else { "" }
  upper(g(0) + g(1))
}

#let avatar(size) = {
  let photo = field(data, "photo")
  if photo != none {
    box(clip: true, radius: 50%, width: size, height: size, image(photo, width: size, height: size, fit: "cover"))
  } else {
    box(radius: 50%, width: size, height: size, fill: navy)[
      #align(center + horizon)[#text(fill: white, size: size * 0.4, weight: "bold")[#initials(field(data, "name", default: ""))]]
    ]
  }
}

// Main-column headings take the sidebar's navy so the two columns echo each
// other: white-on-navy in the sidebar, navy-on-white here.
#let heading-main(title) = block(above: gap-section, below: u)[
  #text(size: fs-h1, weight: "black", fill: navy)[#title]
]

#let heading-side(title) = block(above: gap-section, below: u)[
  #text(size: fs-h3, weight: "bold", fill: side-text)[#title]
]

// Small uppercase sidebar label above a value.
#let side-field(label, value) = if value != none {
  block(above: gap-entry)[
    #text(size: fs-regular, fill: side-label, tracking: 1pt)[#upper(label)]
    #linebreak()
    #text(size: fs-regular, fill: side-text)[#value]
  ]
}

// A rating bar (level out of `max`) drawn with two adjacent fractional columns.
#let rating(level, max: 5) = {
  let f = calc.max(0, calc.min(level, max)) / max
  block(above: 4pt, below: 2pt, width: 100%)[
    #grid(
      columns: (f * 1fr, (1 - f) * 1fr), rows: 3pt, gutter: 0pt,
      rect(width: 100%, height: 100%, fill: side-text),
      rect(width: 100%, height: 100%, fill: bar-track),
    )
  ]
}

// ===========================================================================
// MAIN COLUMN
// ===========================================================================
#let main-column = {
  block(below: u)[
    #grid(
      columns: (auto, 1fr), column-gutter: 5mm, align: horizon,
      avatar(18mm),
      [
        #text(size: fs-name, weight: "black", fill: rgb("#1a1a1a"))[#field(data, "name", default: "")]
        #let t = field(data, "title")
        #if t != none [
          #linebreak()
          #text(size: fs-regular, fill: muted, tracking: 1.5pt)[#upper(subst(t))]
        ]
      ],
    )
  ]

  let summary = field(data, "summary")
  if summary != none {
    heading-main("Professional Summary")
    par(justify: true)[#subst(summary)]
  }

  let techskills = field(data, "technical_skills", default: ())
  if techskills.len() > 0 {
    heading-main("Technical skills & keywords")
    for (i, g) in techskills.enumerate() {
      block(above: if i == 0 { u } else { gap-entry }, below: 0em)[
        #text(size: fs-regular, weight: "bold")[#field(g, "category", default: "")]
      ]
      let items = field(g, "items", default: ())
      if items.len() > 0 { block(above: u)[#items.join(", ")] }
    }
  }

  let experience = field(data, "experience", default: ())
  if experience.len() > 0 {
    heading-main("Employment History")
    for (i, job) in experience.enumerate() {
      block(above: if i == 0 { u } else { gap-entry }, below: 0em)[
        #text(size: fs-h2, weight: "bold")[
          #field(job, "position", default: "")#{
            let c = field(job, "company")
            let l = field(job, "location")
            if c != none [, #c]
            if l != none [, #l]
          }
        ]
      ]
      let (s, e) = (field(job, "start"), field(job, "end"))
      if s != none or e != none {
        block(above: u, below: 0em)[#text(size: fs-regular, fill: muted, tracking: 0.8pt)[#upper[#s — #e]]]
      }
      // Free-form Markdown (paragraphs + nested bullet lists).
      let desc = field(job, "description")
      if desc != none { block(above: gap-entry)[#md-body(desc)] }
    }
  }

  // Each item is a plain string or a { title, description } pair.
  let achievements = field(data, "key_achievements", default: ())
  if achievements.len() > 0 {
    heading-main("Key Achievements")
    let items = ()
    for a in achievements {
      if type(a) == dictionary {
        let t = field(a, "title")
        let d = field(a, "description")
        if t != none and d != none { items.push([*#subst(t)* — #subst(d)]) } else if t != none { items.push([*#subst(t)*]) } else if d != none { items.push(subst(d)) }
      } else {
        items.push(subst(a))
      }
    }
    if items.len() > 0 { list(..items, marker: ([•]), spacing: item-gap, body-indent: 0.4em) }
  }

  // Education. Same layout as Employment History.
  let education = field(data, "education", default: ())
  if education.len() > 0 {
    heading-main("Education")
    for (i, ed) in education.enumerate() {
      block(above: if i == 0 { u } else { gap-entry }, below: 0em)[
        #text(size: fs-h2, weight: "bold")[
          #field(ed, "degree", default: "")#{
            let inst = field(ed, "institution")
            let l = field(ed, "location")
            if inst != none [, #inst]
            if l != none [, #l]
          }
        ]
      ]
      let (s, e) = (field(ed, "start"), field(ed, "end"))
      if s != none or e != none {
        block(above: u, below: 0em)[#text(size: fs-regular, fill: muted, tracking: 0.8pt)[#upper[#s — #e]]]
      }
      let edesc = field(ed, "description")
      if edesc != none { block(above: gap-entry)[#md-body(edesc)] }
    }
  }

  let certs = field(data, "certifications", default: ())
  if certs.len() > 0 {
    heading-main("Certifications")
    for (i, c) in certs.enumerate() {
      block(above: if i == 0 { u } else { gap-entry }, below: 0em)[
        #text(size: fs-h2, weight: "bold")[#field(c, "name", default: "")]
      ]
      let parts = ()
      if field(c, "issuer") != none { parts.push(field(c, "issuer")) }
      if field(c, "date") != none { parts.push(str(field(c, "date"))) }
      if parts.len() > 0 {
        block(above: u)[#text(size: fs-regular, fill: muted)[#parts.join(" · ")]]
      }
      let url = field(c, "url")
      if url != none { block(above: u)[#link(url)[#text(size: fs-regular, fill: accent)[#url]]] }
    }
  }

  let projects = field(data, "projects", default: ())
  if projects.len() > 0 {
    heading-main("Projects & Open-source")
    for (i, p) in projects.enumerate() {
      block(above: if i == 0 { u } else { gap-entry }, below: 0em)[
        #text(size: fs-h2, weight: "bold")[#field(p, "name", default: "")]
        #let tech = field(p, "tech")
        #if tech != none [#text(size: fs-regular, fill: muted)[#" · "#tech]]
      ]
      let d = field(p, "description")
      if d != none { block(above: u)[#subst(d)] }
      let url = field(p, "url")
      if url != none { block(above: u)[#link(url)[#text(size: fs-regular, fill: accent)[#url]]] }
    }
  }
}

// ===========================================================================
// SIDEBAR
// ===========================================================================
#let sidebar = {
  set text(fill: side-text, size: fs-regular)

  let d = field(data, "details", default: (:))
  if d != (:) {
    heading-side("Details")
    let loc = field(d, "location")
    let phone = field(d, "phone")
    let email = field(d, "email")
    let contact = ()
    if loc != none { contact.push([#loc]) }
    if phone != none { contact.push(link("tel:" + phone)[#text(fill: side-text)[#phone]]) }
    if email != none { contact.push(link("mailto:" + email)[#underline(text(fill: side-text)[#email])]) }
    if contact.len() > 0 { block(above: u)[#contact.join(linebreak())] }
    side-field("Nationality", field(d, "nationality"))
    side-field("Driving license", field(d, "driving_license"))
    let bd = field(data, "birth_date")
    let bp = field(data, "birth_place")
    if bd != none or bp != none {
      block(above: gap-entry)[
        #text(size: fs-regular, fill: side-label, tracking: 1pt)[#upper[Date / Place of birth]]
        #if bd != none [ #linebreak() #text(size: fs-regular)[#bd] ]
        #if bp != none [ #linebreak() #text(size: fs-regular)[#bp] ]
      ]
    }
  }

  let links = field(data, "links", default: ())
  if links.len() > 0 {
    heading-side("Links")
    for l in links {
      block(above: u)[#link(field(l, "url", default: "#"))[#underline(text(fill: side-text)[#field(l, "label", default: "")])]]
    }
  }

  let skills = field(data, "skills", default: ())
  if skills.len() > 0 {
    heading-side("Skills")
    for (i, s) in skills.enumerate() {
      block(above: if i == 0 { u } else { gap-entry })[
        #text(size: fs-regular)[#field(s, "name", default: "")]
        #rating(field(s, "level", default: 10), max: 10) // 1-10 scale
      ]
    }
  }

  // level is a free-text proficiency label (e.g. "Native / C2", "B2").
  let langs = field(data, "languages", default: ())
  if langs.len() > 0 {
    heading-side("Languages")
    for (i, l) in langs.enumerate() {
      block(above: if i == 0 { u } else { gap-entry })[
        #text(size: fs-regular)[#field(l, "name", default: "")]
        #let lvl = field(l, "level")
        #if lvl != none [
          #linebreak()
          #text(size: fs-regular, fill: side-label)[#lvl]
        ]
      ]
    }
  }

  let hobbies = field(data, "hobbies")
  if hobbies != none {
    heading-side("Interests")
    block(above: u)[#hobbies]
  }
}

// ===========================================================================
// LAYOUT: two columns, main breaks across pages, sidebar over the navy panel.
// ===========================================================================
#grid(
  columns: (1fr, sidebar-w),
  gutter: 0pt,
  pad(left: 15mm, right: 9mm, top: 14mm, bottom: 12mm, main-column),
  pad(left: 8mm, right: 8mm, top: 14mm, bottom: 12mm, sidebar),
)
