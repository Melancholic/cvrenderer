// Shared data + computed-variable logic for all CV templates. Each template
// imports this with `#import "_common.typ": *` and only defines its own layout;
// presentation (palette, spacing, Markdown markers) stays in the template.
//
// The data path is passed with `--input data=/data/<name>.yaml` (root-absolute).
#let data = yaml(sys.inputs.at("data", default: "/data/cv-example.yaml"))

// Safe optional field access.
#let field(d, k, default: none) = if type(d) == dictionary and k in d { d.at(k) } else { default }

// ---- computed variables ---------------------------------------------------
// A `vars:` map in the YAML lets you reference values in any text via {{name}}.
// Each var is either a literal, `{ since_year: N }` → (current year − N), or
// `{ age_from: <date> }` → age in whole years. `{{age}}` (from the global
// `birth_date`) and `{{experience_years}}` are exposed automatically. Dates may
// be "DD.MM.YYYY" or "YYYY-MM-DD".
#let today = datetime.today()
#let today-year = today.year()

// Parse a date string into (year, month, day), or none if unrecognised.
#let parse-date(s) = {
  if type(s) != str {
    none
  } else if "-" in s {
    let p = s.split("-")
    if p.len() == 3 { (year: int(p.at(0)), month: int(p.at(1)), day: int(p.at(2))) } else { none }
  } else if "." in s {
    let p = s.split(".")
    if p.len() == 3 { (year: int(p.at(2)), month: int(p.at(1)), day: int(p.at(0))) } else { none }
  } else {
    none
  }
}

// Whole-years age from a birth-date string, or none.
#let compute-age(bd) = {
  let d = parse-date(bd)
  if d == none {
    none
  } else {
    let age = today-year - d.year
    let before-birthday = today.month() < d.month or (today.month() == d.month and today.day() < d.day)
    if before-birthday { age - 1 } else { age }
  }
}

// Month number from a name like "Jun"/"June"/"Sept" (first 3 letters), or none.
#let month-num(tok) = {
  let m = (
    jan: 1, feb: 2, mar: 3, apr: 4, may: 5, jun: 6,
    jul: 7, aug: 8, sep: 9, oct: 10, nov: 11, dec: 12,
  )
  m.at(lower(tok).slice(0, calc.min(3, tok.len())), default: none)
}

// Absolute month index (year*12 + month-1) for "Mon YYYY", "YYYY", or an
// open-ended end like "Present"/"Now"/"Current". none if unparseable.
#let month-index(s) = {
  if type(s) != str {
    none
  } else if lower(s.trim()) in ("present", "current", "now", "") {
    today-year * 12 + (today.month() - 1)
  } else {
    let parts = s.trim().split(" ").filter(p => p != "")
    if parts.len() == 2 and month-num(parts.at(0)) != none {
      int(parts.at(1)) * 12 + (month-num(parts.at(0)) - 1)
    } else if parts.len() == 1 and parts.at(0).match(regex("^[0-9]{4}$")) != none {
      int(parts.at(0)) * 12
    } else {
      none
    }
  }
}

// Total worked months, counting concurrent jobs once: sort the per-job month
// intervals, merge overlapping/touching ones, sum the merged spans (gaps out).
#let experience-months() = {
  let ivals = ()
  for job in field(data, "experience", default: ()) {
    let s = month-index(field(job, "start"))
    let e = month-index(field(job, "end"))
    if s != none and e != none and e > s { ivals.push((s, e)) }
  }
  if ivals.len() == 0 {
    0
  } else {
    let sorted = ivals.sorted(key: iv => iv.at(0))
    let total = 0
    let cur = sorted.at(0)
    for iv in sorted.slice(1) {
      if iv.at(0) <= cur.at(1) {
        cur = (cur.at(0), calc.max(cur.at(1), iv.at(1))) // overlap/touch: extend
      } else {
        total += cur.at(1) - cur.at(0) // disjoint: bank the current span
        cur = iv
      }
    }
    total + (cur.at(1) - cur.at(0))
  }
}

#let vars = (:)
#{
  for (k, v) in field(data, "vars", default: (:)).pairs() {
    if type(v) == dictionary and "since_year" in v {
      vars.insert(k, str(today-year - v.at("since_year")))
    } else if type(v) == dictionary and "age_from" in v {
      let a = compute-age(v.at("age_from"))
      if a != none { vars.insert(k, str(a)) }
    } else {
      vars.insert(k, str(v))
    }
  }
  // Auto {{experience_years}}: whole years, with a trailing "+" when the
  // leftover months are >= 6 (10y6m -> "10+", 10y5m -> "10").
  if "experience_years" not in vars {
    let m = experience-months()
    if m > 0 {
      let years = int(m / 12)
      let plus = if m - years * 12 >= 6 { "+" } else { "" }
      vars.insert("experience_years", str(years) + plus)
    }
  }
  // Auto {{age}} from the global birth_date (unless overridden).
  if "age" not in vars {
    let a = compute-age(field(data, "birth_date"))
    if a != none { vars.insert("age", str(a)) }
  }
}

// Replace every {{name}} placeholder in a string with its computed value.
#let subst(s) = {
  if type(s) != str {
    s
  } else {
    let out = s
    for (k, v) in vars.pairs() { out = out.replace("{{" + k + "}}", v) }
    out
  }
}
