"""Generate synthetic FHIR R4 data for the IBD dataset (cohort + export query).

Usage (from this folder):  py -3 generate_ibd_synthetic.py [--patients 60] [--seed 42] [--today 2026-10-28]

Writes to ./output:
  Group.json       the cohort of the dataset, as in the IG (Group-ibd-dataset-cohort)
  Parameters.json  the export request that describes the dataset: _type, _typeFilter, _elements
                   (Parameters-ibd-dataset-export)
  source/          resources as the hospital holds them (SAN profiles, with identifiers)
                   Patient, EpisodeOfCare, Condition, Procedure, Observation (.ndjson)
  export/          what the $export would release: only the _elements of Parameters.json, for the
                   members of Group.json (Patient, Procedure, Observation .ndjson, not de-identified)

Every value is random; there is no real patient data. The script includes deliberate
negative cases (minor, wrong zorgtype, wrong diagnosis, episode before 2018, inactive
episode) so the cohort criteria can be tested in both directions.
"""
import argparse
import json
import random
import uuid
from datetime import date, datetime, timedelta
from pathlib import Path

BASE = "https://ig.santeon.nl/sim-on-fhir"
# DBC code systems, OIDs from the DBC-Onderhoud HL7v3 DBC-grouper implementation guide
# (root 2.16.840.1.113883.2.4.3.27 = DBC Onderhoud): .15.2 zorgtype, .15.4.<specialisme> diagnose, .15.5 zorgactiviteit
DBC_OID = "urn:oid:2.16.840.1.113883.2.4.3.27.15"
SYS_ZORGTYPE = f"{DBC_OID}.2"
SYS_SPECDIAG = f"{DBC_OID}.4"  # the specialisme code is appended: .4.0313
# Zorgactiviteit = Nictiz "NZa codes" as used by zib Verrichting and e.g. the RIVO-Noord Zorgviewer IG
SYS_NZA_ACT = f"{DBC_OID}.5"
SYS_ID = "https://ig.santeon.nl/sim-on-fhir/NamingSystem/identificatienummer"
SCT = "http://snomed.info/sct"
LOINC = "http://loinc.org"
UCUM = "http://unitsofmeasure.org"
NULLFLAVOR = "http://terminology.hl7.org/CodeSystem/v3-NullFlavor"

TODAY = date(2026, 10, 28)
COHORT_START = date(2018, 1, 1)

# (code, display); the code is "<specialisme>|<diagnose>" as in local-specialisme-diagnose-codes
# Displays from HipsETL valuesets (Specialisme_DiagnoseCodelijst and ZorgTypeCode, NZa)
IBD_DIAG = [
    ("0313|922", "crohn"),
    ("0313|923", "colitis ulcerosa"),
    ("0318|601", "morbus crohn"),
    ("0318|602", "colitis ulcerosa"),
]
# Diagnoses unrelated to IBD (real codes and displays from HipsETL); none is in local-specialisme-diagnose-codes.
# 0313|006 and 0318|104/101 are deliberate near-misses: same specialisms as the IBD diagnoses.
NON_IBD_DIAG = [
    ("0313|006", "analyse buikklachten zonder diagnose"),
    ("0313|007", "analyse dyspnoe zonder diagnose"),
    ("0318|101", "gastroduodenoscopie"),
    ("0318|104", "colonoscopie"),
    ("0320|04", "ap, ischemie aangetoond"),
    ("0320|13", "follow up na myocardinfarct"),
    ("0301|155", "refractie-anomalie"),
    ("0330|0201", "neoplasma intracerebraal"),
]
ZORGTYPE_OK = [("11", "Reguliere Zorg"), ("21", "Vervolg")]
ZORGTYPE_NOT_OK = [("13", "ICC")]
# Displays from HipsETL valuesets/valueset_VerrichtingTypeNZaCodelijst.csv (NZa)
SCOPIE = [("034620", "Diagnostische endoscopie van oesofagus, maag en/of duodenum met behulp van een flexibele endoscoop, inclusief eventuele biopten."),
          ("034686", "DIAGN. COLOSCOPIE MET FIBERSC."),
          ("034690", "Diagnostische sigmoidoscopie met behulp van fiberscoop, inclusief eventuele proefexcisie(s) en poliepectomie."),
          ("035582", "Diagnostische laparoscopie, inclusief eventuele proefexcisie(s) (excl. bij vruchtbaarheidsproblematiek, zie 035583).")]
SCOPIE_CODES = {c for c, _ in SCOPIE}  # = local-verrichting-codes-nza
OTHER_NZA = [("087090", "MRI ABDOMEN"), ("190204", "VERPLEEGDAG")]
ALCOHOL = [(SCT, "219006", "drinkt alcohol"),
           (SCT, "105542008", "drinkt momenteel geen alcohol"),
           (SCT, "82581004", "dronk vroeger alcohol"),
           (SCT, "783261004", "levenslange niet-drinker van alcohol"),
           (NULLFLAVOR, "OTH", "Anders")]


def last_sunday(year, month):
    d = date(year, month, 31)
    return d - timedelta(days=(d.weekday() + 1) % 7)


def iso(dt_):
    """ISO dateTime with the Europe/Amsterdam offset: +02:00 between the last Sundays of March and October."""
    summer = last_sunday(dt_.year, 3) <= dt_.date() < last_sunday(dt_.year, 10)
    return dt_.strftime("%Y-%m-%dT%H:%M:%S") + ("+02:00" if summer else "+01:00")


def uid(rng):
    return str(uuid.UUID(int=rng.getrandbits(128), version=4))


def rand_date(rng, start, end):
    return start + timedelta(days=rng.randint(0, (end - start).days))


def dt(d, rng):
    return iso(datetime(d.year, d.month, d.day, rng.randint(7, 17), rng.choice([0, 15, 30, 45])))


def ref(rtype, rid):
    return {"reference": f"{rtype}/{rid}"}


def make_patient(rng, n, minor):
    age_days = rng.randint(12 * 365, 17 * 365) if minor else rng.randint(19 * 365, 85 * 365)
    pid = uid(rng)
    return {
        "resourceType": "Patient",
        "id": pid,
        "identifier": [{"system": SYS_ID, "value": f"P{100000 + n}"}],
        "gender": rng.choice(["male", "female"]),
        "birthDate": (TODAY - timedelta(days=age_days)).isoformat(),
    }


def make_episode(rng, patient, code, zorgtype, start, status, n, end=None):
    cond_id, eoc_id = uid(rng), uid(rng)
    condition = {
        "resourceType": "Condition",
        "id": cond_id,
        "identifier": [{"system": SYS_ID, "value": f"PR{n}"}],
        "code": {"coding": [{"system": f"{SYS_SPECDIAG}.{code[0].split('|')[0]}",
                             "code": code[0].split("|")[1], "display": code[1]}]},
        "subject": ref("Patient", patient["id"]),
    }
    period = {"start": start.isoformat()}
    if status in ("finished", "cancelled"):
        period["end"] = (end or min(TODAY, start + timedelta(days=rng.randint(60, 700)))).isoformat()
    episode = {
        "resourceType": "EpisodeOfCare",
        "id": eoc_id,
        "identifier": [{"system": SYS_ID, "value": f"DBC{n}"}],
        "status": status,
        "type": [{"coding": [{"system": SYS_ZORGTYPE, "code": zorgtype[0], "display": zorgtype[1]}]}],
        "diagnosis": [{"condition": ref("Condition", cond_id)}],
        "patient": ref("Patient", patient["id"]),
        "period": period,
    }
    return episode, condition


def make_procedure(rng, patient, code, when, n):
    start = datetime(when.year, when.month, when.day, rng.randint(8, 16), 0)
    end = start + timedelta(minutes=rng.randint(20, 60))
    return {
        "resourceType": "Procedure",
        "id": uid(rng),
        "identifier": [{"system": SYS_ID, "value": f"V{n}"}],
        "status": "completed",
        "code": {"coding": [{"system": SYS_NZA_ACT, "code": code[0], "display": code[1]}]},
        "subject": ref("Patient", patient["id"]),
        "performedPeriod": {"start": iso(start), "end": iso(end)},
    }


def make_calprotectin(rng, patient, when, n):
    # IBD-typical skew: many low values, some high; occasionally a "> 600" result
    value = round(rng.choice([rng.uniform(5, 50), rng.uniform(50, 250), rng.uniform(250, 1800)]), 0)
    quantity = {"value": value, "unit": "ug/g", "system": UCUM, "code": "ug/g"}
    if value > 1500:
        quantity.update({"value": 1500, "comparator": ">"})
    return {
        "resourceType": "Observation",
        "id": uid(rng),
        "identifier": [{"system": SYS_ID, "value": f"M{n}"}],
        "status": "final",
        "code": {"coding": [{"system": LOINC, "code": "38445-3", "display": "Calprotectin [Mass/volume] in Stool"}]},
        "subject": ref("Patient", patient["id"]),
        "effectiveDateTime": dt(when, rng),
        "valueQuantity": quantity,
    }


def make_alcohol(rng, patient, when, n):
    system, code, display = rng.choice(ALCOHOL)
    return {
        "resourceType": "Observation",
        "id": uid(rng),
        "identifier": [{"system": SYS_ID, "value": f"M{n}"}],
        "status": "final",
        "code": {"coding": [{"system": SCT, "code": "228273003",
                             "display": "Finding relating to alcohol drinking behavior (finding)"}]},
        "subject": ref("Patient", patient["id"]),
        "effectiveDateTime": dt(when, rng),
        "valueCodeableConcept": {"coding": [{"system": system, "code": code, "display": display}]},
    }


def build(n_patients, seed):
    rng = random.Random(seed)
    res = {k: [] for k in ("Patient", "EpisodeOfCare", "Condition", "Procedure", "Observation")}
    expected = []
    counter = 0

    def nxt():
        nonlocal counter
        counter += 1
        return counter

    for i in range(n_patients):
        # scenario mix: ~70% in cohort, the rest are negative cases
        scenario = rng.choices(
            ["in", "minor", "zorgtype", "diagnose", "before2018", "inactive"],
            weights=[70, 6, 6, 6, 6, 6])[0]
        patient = make_patient(rng, i, minor=(scenario == "minor"))
        res["Patient"].append(patient)

        born = date.fromisoformat(patient["birthDate"])
        # an adult DBC (internal medicine / MDL) starts at 18 or later; the minor test case at 10 or later
        earliest = max(COHORT_START, born + timedelta(days=(10 if scenario == "minor" else 18) * 365 + 5))

        # IBD care: a first DBC (zorgtype 11) followed by 0-2 follow-up DBCs (zorgtype 21), same diagnosis.
        # Closed DBCs are finished; only the last one is still open.
        episodes = []
        if scenario != "diagnose":
            if scenario == "before2018":
                first, limit = rand_date(rng, date(2014, 1, 1), date(2016, 12, 31)), date(2017, 12, 31)
            else:
                first, limit = rand_date(rng, earliest, TODAY - timedelta(days=200)), TODAY - timedelta(days=30)
            starts = [first]
            for _ in range(rng.choice([0, 0, 1, 2])):
                following = starts[-1] + timedelta(days=rng.randint(120, 400))
                if following > limit:
                    break
                starts.append(following)
            diag = rng.choice(IBD_DIAG)
            for k, begin in enumerate(starts):
                zt = rng.choice(ZORGTYPE_NOT_OK) if scenario == "zorgtype" else                     ZORGTYPE_OK[0 if k == 0 else 1]
                if k < len(starts) - 1:
                    status, end = "finished", starts[k + 1] - timedelta(days=1)
                else:
                    status, end = ("finished", None) if scenario == "inactive" else ("active", None)
                episodes.append(make_episode(rng, patient, diag, zt, begin, status, nxt(), end))

        # Care that has nothing to do with IBD: other diagnoses, regular zorgtype, mostly in the same specialisms.
        for _ in range(rng.choice([1, 2, 2]) if scenario == "diagnose" else rng.choice([0, 1, 1, 2])):
            begin = rand_date(rng, earliest, TODAY - timedelta(days=100))
            status = rng.choice(["active", "finished"])
            episodes.append(make_episode(rng, patient, rng.choice(NON_IBD_DIAG), rng.choice(ZORGTYPE_OK),
                                         begin, status, nxt()))

        for episode, condition in episodes:
            res["EpisodeOfCare"].append(episode)
            res["Condition"].append(condition)
        start = min(date.fromisoformat(e["period"]["start"]) for e, _ in episodes)

        # Procedures and measurements are generated for every patient, also for the
        # negative cases: the export filters on the Group, not on the resource itself.
        scopie_dates = []
        for _ in range(rng.choice([0, 1, 1, 2, 3])):
            when = rand_date(rng, max(start, COHORT_START), TODAY - timedelta(days=30))
            scopie_dates.append(when)
            res["Procedure"].append(make_procedure(rng, patient, rng.choice(SCOPIE), when, nxt()))
        if rng.random() < 0.3:
            res["Procedure"].append(make_procedure(
                rng, patient, rng.choice(OTHER_NZA), rand_date(rng, COHORT_START, TODAY), nxt()))

        # K3.6.3: calprotectin within 90 days before ~60% of the scopies (some exactly on day 0 / 90)
        for when in scopie_dates:
            if rng.random() < 0.6:
                offset = rng.choice([0, 90, rng.randint(1, 89)])
                res["Observation"].append(make_calprotectin(rng, patient, when - timedelta(days=offset), nxt()))
            elif rng.random() < 0.3:
                # deliberately outside the window (91+ days before)
                res["Observation"].append(
                    make_calprotectin(rng, patient, when - timedelta(days=rng.randint(91, 300)), nxt()))
        if rng.random() < 0.2:
            res["Observation"].append(make_calprotectin(rng, patient, rand_date(rng, COHORT_START, TODAY), nxt()))

        # alcohol status recorded for ~65% of the patients
        if rng.random() < 0.65:
            res["Observation"].append(make_alcohol(rng, patient, rand_date(rng, COHORT_START, TODAY), nxt()))

        expected.append({
            "patient": patient["id"],
            "identifier": patient["identifier"][0]["value"],
            "inCohort": scenario == "in",
            "episodes": len(episodes),
            "reason": {"in": "all criteria met",
                       "minor": "younger than 18 at delivery (Patient.birthdate le today-18y)",
                       "zorgtype": "zorgtype not in 11/21",
                       "diagnose": "specialisme-diagnose not in IBD value set",
                       "before2018": "episode period.start before 2018-01-01 (the IBD.sql intent; note that a FHIR "
                                     "date=ge2018-01-01 search also matches an open Period that started earlier)",
                       "inactive": "EpisodeOfCare.status is not active"}[scenario],
        })
    return res, expected


def code_list(system, codes):
    """FHIR token list: system|code,system|code"""
    return ",".join(f"{system}|{c}" for c in codes)


def ibd_diagnosis_codes():
    """The four IBD specialisme-diagnose combinations; the specialisme is part of the system."""
    return ",".join(code_list(f"{SYS_SPECDIAG}.{spec}", [diag]) for spec, diag in
                    (c.split("|") for c, _ in IBD_DIAG))


def cohort_group():
    """The Group as generated in the IG (input/fsh/generated/Exports.fsh), with today-18y resolved."""
    cutoff = date(TODAY.year - 18, TODAY.month, TODAY.day).isoformat()
    filters = [
        f"Patient?birthdate=le{cutoff}",
        f"EpisodeOfCare?type={code_list(SYS_ZORGTYPE, [c for c, _ in ZORGTYPE_OK])}"
        f"&status=active&date=ge2018-01-01&diagnosis:Condition.code={ibd_diagnosis_codes()}",
    ]
    return {
        "resourceType": "Group", "id": "ibd-dataset-cohort", "type": "person", "actual": True,
        "name": "IBD cohort",
        "modifierExtension": [{
            "url": "http://hl7.org/fhir/uv/bulkdata/StructureDefinition/member-filter",
            "valueExpression": {"language": "application/x-fhir-query", "expression": f}} for f in filters],
    }


def export_parameters():
    """The Parameters as generated in the IG (input/fsh/generated/Exports.fsh)."""
    def string(name, value):
        return {"name": name, "valueString": value}
    return {
        "resourceType": "Parameters", "id": "ibd-dataset-export",
        "parameter": [
            string("_type", "Procedure,Observation,Patient"),
            string("_typeFilter", f"Procedure?code={code_list(SYS_NZA_ACT, sorted(SCOPIE_CODES))}&date=ge2018-01-01"),
            string("_typeFilter", "Observation?code=http://loinc.org|38445-3,http://snomed.info/sct|228273003"
                                  "&date=ge2018-01-01"),
            string("_typeFilter", "Patient?gender=male,female"),
            string("_elements", "Procedure.code,Procedure.performed[x],Procedure.subject,Observation.code,"
                                "Observation.value[x],Observation.effective[x],Observation.subject,"
                                "Patient.gender,Patient.birthDate"),
        ],
    }


def export_view(res, in_cohort_ids):
    """Only the _elements of Parameters-ibd-dataset-export, for cohort members."""
    out = {"Patient": [], "Procedure": [], "Observation": []}
    for p in res["Patient"]:
        if p["id"] in in_cohort_ids:
            out["Patient"].append({"resourceType": "Patient", "id": p["id"],
                                   "gender": p["gender"], "birthDate": p["birthDate"]})
    for pr in res["Procedure"]:
        if pr["subject"]["reference"].split("/")[1] in in_cohort_ids and \
                pr["code"]["coding"][0]["code"] in SCOPIE_CODES and \
                pr["performedPeriod"]["start"][:10] >= COHORT_START.isoformat():
            # status is 1..1 in R4 and therefore always returned, also when it is not in _elements
            out["Procedure"].append({k: pr[k] for k in ("resourceType", "id", "status", "code", "performedPeriod", "subject")})
    for ob in res["Observation"]:
        if ob["subject"]["reference"].split("/")[1] in in_cohort_ids and \
                ob["effectiveDateTime"][:10] >= COHORT_START.isoformat():
            out["Observation"].append({k: v for k, v in ob.items() if k != "identifier"})
    return out


def write_ndjson(path, items):
    path.write_text("\n".join(json.dumps(i, ensure_ascii=False) for i in items) + "\n", encoding="utf-8")


def main():
    global TODAY
    ap = argparse.ArgumentParser()
    ap.add_argument("--patients", type=int, default=60)
    ap.add_argument("--seed", type=int, default=42)
    ap.add_argument("--today", default=TODAY.isoformat(), help="delivery date; the 18-year cut-off is relative to it")
    args = ap.parse_args()
    TODAY = date.fromisoformat(args.today)

    res, expected = build(args.patients, args.seed)
    members = {e["patient"] for e in expected if e["inCohort"]}

    out = Path(__file__).parent / "output"
    for sub in ("source", "export"):
        (out / sub).mkdir(parents=True, exist_ok=True)

    (out / "Group.json").write_text(json.dumps(cohort_group(), indent=2, ensure_ascii=False), encoding="utf-8")
    (out / "Parameters.json").write_text(json.dumps(export_parameters(), indent=2, ensure_ascii=False), encoding="utf-8")

    for rtype, items in res.items():
        write_ndjson(out / "source" / f"{rtype}.ndjson", items)
    for rtype, items in export_view(res, members).items():
        write_ndjson(out / "export" / f"{rtype}.ndjson", items)

    print(f"patients: {len(res['Patient'])} (in cohort: {len(members)})")
    for rtype in ("EpisodeOfCare", "Condition", "Procedure", "Observation"):
        print(f"{rtype}: {len(res[rtype])}")
    print(f"written to {out}")


if __name__ == "__main__":
    main()
