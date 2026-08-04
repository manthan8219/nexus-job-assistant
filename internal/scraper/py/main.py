"""
Career page scraper microservice.
Supports multiple backends: scrapegraphai | crawl4ai | playwright

Backend auto-detected from what's installed, or forced via SCRAPER_BACKEND env var.
Run: uvicorn main:app --port 8765
"""

import os, logging, importlib
from typing import Optional
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

logging.basicConfig(level=logging.INFO)
log = logging.getLogger(__name__)

app = FastAPI(title="Career Scraper", version="2.0.0")

# ── Config ────────────────────────────────────────────────────────────────────

OLLAMA_BASE_URL = os.getenv("OLLAMA_BASE_URL", "http://localhost:11434")
OLLAMA_MODEL    = os.getenv("OLLAMA_MODEL", "llama3.2")
EMBED_MODEL     = os.getenv("EMBED_MODEL", "nomic-embed-text")

def _available(pkg: str) -> bool:
    try:
        importlib.import_module(pkg)
        return True
    except ImportError:
        return False

def _detect_backend() -> str:
    forced = os.getenv("SCRAPER_BACKEND", "").lower()
    if forced:
        return forced
    if _available("scrapegraphai"):
        return "scrapegraphai"
    if _available("crawl4ai"):
        return "crawl4ai"
    if _available("playwright"):
        return "playwright"
    raise RuntimeError("No scraper backend installed. Install one via the app.")

BACKEND = _detect_backend()
log.info(f"Using backend: {BACKEND}")

# ── Models ────────────────────────────────────────────────────────────────────

class ScrapeRequest(BaseModel):
    url: str
    company: str
    title_keywords: list[str] = []

class JobResult(BaseModel):
    title: str
    company: str
    location: str
    department: str
    apply_url: str
    remote: bool

class ScrapeResponse(BaseModel):
    company: str
    url: str
    jobs: list[JobResult]
    backend: str
    error: Optional[str] = None

class BatchScrapeRequest(BaseModel):
    targets: list[ScrapeRequest]

class BatchScrapeResponse(BaseModel):
    results: list[ScrapeResponse]

class LinkedInScrapeRequest(BaseModel):
    keywords: str
    location: str = ""
    max_pages: int = 3          # each page = 25 results
    easy_apply_only: bool = True

class LinkedInScrapeResponse(BaseModel):
    jobs: list[JobResult]
    total_found: int
    error: Optional[str] = None

# ── Helpers ───────────────────────────────────────────────────────────────────

SCRAPE_PROMPT = """
Extract all job listings from this career page.
Return a JSON object: {"jobs": [{"title":"","location":"","department":"","apply_url":"","remote":false}]}
Return ONLY the JSON, no explanation.
"""

def _matches(title: str, keywords: list[str]) -> bool:
    if not keywords:
        return True
    t = title.lower()
    return any(k.lower() in t for k in keywords)

def _normalize(raw: dict, company: str, base_url: str = "") -> JobResult:
    url = str(raw.get("apply_url", "")).strip()
    if url and not url.startswith("http") and base_url:
        from urllib.parse import urljoin
        url = urljoin(base_url, url)
    return JobResult(
        title=str(raw.get("title", "")).strip(),
        company=company,
        location=str(raw.get("location", "")).strip(),
        department=str(raw.get("department", "")).strip(),
        apply_url=url,
        remote=bool(raw.get("remote", False)),
    )

# ── Backend: ScrapeGraphAI ────────────────────────────────────────────────────

def _scrape_scrapegraphai(req: ScrapeRequest) -> list[JobResult]:
    from scrapegraphai.graphs import SmartScraperGraph
    cfg = {
        "llm": {"model": f"ollama/{OLLAMA_MODEL}", "base_url": OLLAMA_BASE_URL, "temperature": 0},
        "embeddings": {"model": f"ollama/{EMBED_MODEL}", "base_url": OLLAMA_BASE_URL},
        "verbose": False,
        "headless": True,
    }
    result = SmartScraperGraph(prompt=SCRAPE_PROMPT, source=req.url, config=cfg).run()
    return [_normalize(j, req.company, req.url) for j in (result.get("jobs", []) if isinstance(result, dict) else [])]

# ── Backend: Crawl4AI ─────────────────────────────────────────────────────────

def _scrape_crawl4ai(req: ScrapeRequest) -> list[JobResult]:
    import asyncio, json
    from crawl4ai import AsyncWebCrawler
    from crawl4ai.extraction_strategy import LLMExtractionStrategy

    strategy = LLMExtractionStrategy(
        provider=f"ollama/{OLLAMA_MODEL}",
        api_base=OLLAMA_BASE_URL,
        instruction=SCRAPE_PROMPT,
        schema={"type": "object", "properties": {"jobs": {"type": "array"}}},
    )

    async def _run():
        async with AsyncWebCrawler(headless=True) as crawler:
            r = await crawler.arun(url=req.url, extraction_strategy=strategy, bypass_cache=True)
            return r.extracted_content

    raw = asyncio.run(_run())
    if isinstance(raw, str):
        raw = json.loads(raw)
    return [_normalize(j, req.company, req.url) for j in (raw.get("jobs", []) if isinstance(raw, dict) else [])]

# ── Backend: Playwright (rule-based, no LLM) ──────────────────────────────────

def _scrape_playwright(req: ScrapeRequest) -> list[JobResult]:
    from playwright.sync_api import sync_playwright

    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page()
        page.goto(req.url, wait_until="networkidle", timeout=30000)
        html = page.content()
        browser.close()

    return _extract_jobs_from_html(html, req.company, req.url)

# _extract_jobs_from_html parses rendered HTML for job-like anchor links.
# Reused by the headless Playwright backend and the CDP (logged-in) board
# scraper so both share one extraction implementation.
def _extract_jobs_from_html(html: str, company: str, base_url: str) -> list[JobResult]:
    import re
    from bs4 import BeautifulSoup

    soup = BeautifulSoup(html, "html.parser")
    seen, jobs = set(), []
    for a in soup.find_all("a", href=True):
        href, text = a["href"].strip(), a.get_text(" ", strip=True)
        if not text or href in seen:
            continue
        if not any(kw in href.lower() for kw in ["/job", "/career", "/position", "/opening", "/role", "/apply"]):
            continue
        # Strip trailing "Learn more" / arrow noise
        text = re.sub(r"\s*Learn more.*$", "", text, flags=re.IGNORECASE).strip()
        text = re.sub(r"\s*→.*$", "", text).strip()
        if not text or len(text) > 120:
            continue
        # Skip pure nav links (exact match to generic words)
        if text.lower() in {"careers", "jobs", "open positions", "apply", "learn more"}:
            continue
        seen.add(href)
        full_url = href if href.startswith("http") else base_url.rstrip("/") + "/" + href.lstrip("/")
        # Extract location: last word-group after known location signals
        location = ""
        loc_match = re.search(r"(North America|Europe|Remote|Worldwide|London|New York|San Francisco|Global)", text, re.IGNORECASE)
        if loc_match:
            location = loc_match.group(1)
            # Remove location from title
            text = text[:loc_match.start()].strip().rstrip(",").strip()
        remote = "remote" in text.lower() or "remote" in location.lower()
        jobs.append(JobResult(title=text, company=company,
                              location=location, department="",
                              apply_url=full_url, remote=remote))
    return jobs

# ── Dispatch ──────────────────────────────────────────────────────────────────

_BACKENDS = {
    "scrapegraphai": _scrape_scrapegraphai,
    "crawl4ai":      _scrape_crawl4ai,
    "playwright":    _scrape_playwright,
}

def _scrape_one(req: ScrapeRequest) -> ScrapeResponse:
    log.info(f"[{BACKEND}] {req.company} → {req.url}")
    fn = _BACKENDS.get(BACKEND)
    if not fn:
        return ScrapeResponse(company=req.company, url=req.url, jobs=[], backend=BACKEND,
                              error=f"unknown backend: {BACKEND}")
    try:
        jobs = [j for j in fn(req) if j.title and _matches(j.title, req.title_keywords)]
        log.info(f"  → {len(jobs)} jobs")
        return ScrapeResponse(company=req.company, url=req.url, jobs=jobs, backend=BACKEND)
    except Exception as e:
        log.error(f"Error: {e}")
        return ScrapeResponse(company=req.company, url=req.url, jobs=[], backend=BACKEND, error=str(e))

# ── Routes ────────────────────────────────────────────────────────────────────

@app.get("/health")
def health():
    return {"status": "ok", "backend": BACKEND, "model": OLLAMA_MODEL}

@app.get("/backends")
def list_backends():
    available = [b for b in ["scrapegraphai", "crawl4ai", "playwright"] if _available(b.replace("-","_"))]
    return {"active": BACKEND, "available": available}

@app.post("/scrape", response_model=ScrapeResponse)
def scrape_one(req: ScrapeRequest):
    return _scrape_one(req)

@app.post("/scrape/batch", response_model=BatchScrapeResponse)
def scrape_batch(req: BatchScrapeRequest):
    if len(req.targets) > 20:
        raise HTTPException(status_code=400, detail="Max 20 targets per batch")
    return BatchScrapeResponse(results=[_scrape_one(t) for t in req.targets])

# ── Board scraper (job boards: headless or logged-in via CDP) ──────────────────

class BoardScrapeRequest(BaseModel):
    url: str
    company: str = ""
    title_keywords: list[str] = []
    # use_session=True connects to the user's already-running Chrome over CDP
    # (launched with --remote-debugging-port=9222) so login-gated boards
    # (Instahyre, Foundit, etc.) are scraped with the user's live session.
    use_session: bool = False

@app.post("/scrape/board", response_model=ScrapeResponse)
def scrape_board(req: BoardScrapeRequest):
    log.info(f"[board] {'session' if req.use_session else 'headless'} → {req.url}")
    try:
        from playwright.sync_api import sync_playwright
    except ImportError:
        return ScrapeResponse(company=req.company, url=req.url, jobs=[],
                              backend="playwright",
                              error="playwright not installed — install via Settings › Career Scraper")
    try:
        with sync_playwright() as p:
            if req.use_session:
                # Connect to the user's live Chrome (must be started with
                # --remote-debugging-port=9222). Reuses the logged-in context.
                try:
                    browser = p.chromium.connect_over_cdp("http://localhost:9222")
                except Exception as e:
                    return ScrapeResponse(company=req.company, url=req.url, jobs=[],
                                          backend="playwright",
                                          error="CDP connect failed — launch Chrome with "
                                                "--remote-debugging-port=9222: " + str(e))
                ctx = browser.contexts[0] if browser.contexts else browser.new_context()
            else:
                browser = p.chromium.launch(headless=True)
                ctx = browser.new_context(
                    user_agent="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                               "AppleWebKit/537.36 (KHTML, like Gecko) "
                               "Chrome/126.0.0.0 Safari/537.36",
                    viewport={"width": 1280, "height": 800},
                )
            page = ctx.new_page()
            try:
                page.goto(req.url, wait_until="networkidle", timeout=20000)
            except Exception:
                # Some SPAs (Angular/React boards) keep the network busy so
                # networkidle never fires; fall back to DOM load + render delay.
                page.goto(req.url, wait_until="domcontentloaded", timeout=20000)
                page.wait_for_timeout(4000)
            html = page.content()
            if not req.use_session:
                browser.close()

        company = req.company or req.url.split("//")[-1].split("/")[0].replace("www.", "")
        jobs = _extract_jobs_from_html(html, company, req.url)
        jobs = [j for j in jobs if j.title and _matches(j.title, req.title_keywords)]
        log.info(f"  → {len(jobs)} jobs")
        return ScrapeResponse(company=company, url=req.url, jobs=jobs, backend="playwright")
    except Exception as e:
        log.error(f"board scrape error: {e}")
        return ScrapeResponse(company=req.company, url=req.url, jobs=[],
                              backend="playwright", error=str(e))


# ── LinkedIn scraper ───────────────────────────────────────────────────────────

def _scrape_linkedin(req: LinkedInScrapeRequest) -> LinkedInScrapeResponse:
    import re, time, random
    try:
        from playwright.sync_api import sync_playwright
        from bs4 import BeautifulSoup
    except ImportError:
        return LinkedInScrapeResponse(jobs=[], total_found=0,
                                      error="playwright not installed — install via Settings › Career Scraper")

    jobs: list[JobResult] = []
    seen: set[str] = set()

    base = "https://www.linkedin.com/jobs/search/"
    params = f"?keywords={req.keywords.replace(' ', '%20')}"
    if req.location:
        params += f"&location={req.location.replace(' ', '%20')}"
    if req.easy_apply_only:
        params += "&f_AL=true"
    params += "&sortBy=DD"  # date descending

    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        ctx = browser.new_context(
            user_agent="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                       "AppleWebKit/537.36 (KHTML, like Gecko) "
                       "Chrome/120.0.0.0 Safari/537.36",
            viewport={"width": 1280, "height": 800},
        )
        page = ctx.new_page()

        for page_num in range(req.max_pages):
            url = base + params + f"&start={page_num * 25}"
            log.info(f"[linkedin] page {page_num + 1}: {url}")
            try:
                page.goto(url, wait_until="domcontentloaded", timeout=20000)
                # wait for job cards to appear
                page.wait_for_selector("ul.jobs-search__results-list, "
                                       ".job-search-card, "
                                       "[data-entity-urn]",
                                       timeout=8000)
            except Exception:
                pass  # still try to parse whatever loaded

            # random human-like delay between pages
            time.sleep(random.uniform(2.5, 5.0))
            html = page.content()
            soup = BeautifulSoup(html, "html.parser")

            # LinkedIn uses two different page layouts depending on auth state
            cards = (soup.select("ul.jobs-search__results-list > li") or
                     soup.select(".job-search-card") or
                     soup.select("[data-entity-urn]"))

            if not cards:
                log.warning(f"[linkedin] no cards on page {page_num + 1}, stopping")
                break

            page_jobs = 0
            for card in cards:
                # ── title ──────────────────────────────────────────────────
                title_el = (card.select_one(".base-search-card__title") or
                            card.select_one("h3") or
                            card.select_one(".job-search-card__title"))
                title = title_el.get_text(strip=True) if title_el else ""
                if not title:
                    continue

                # ── company ────────────────────────────────────────────────
                co_el = (card.select_one(".base-search-card__subtitle") or
                         card.select_one(".job-search-card__company-name") or
                         card.select_one("h4"))
                company = co_el.get_text(strip=True) if co_el else ""

                # ── location ───────────────────────────────────────────────
                loc_el = (card.select_one(".job-search-card__location") or
                          card.select_one(".base-search-card__metadata span"))
                location = loc_el.get_text(strip=True) if loc_el else ""

                # ── URL ────────────────────────────────────────────────────
                link_el = card.select_one("a[href*='/jobs/view/']")
                if not link_el:
                    link_el = card.select_one("a[href]")
                href = link_el["href"] if link_el else ""
                # strip query params / tracking after the job ID
                clean_url = re.sub(r"\?.*$", "", href.strip())
                if not clean_url or clean_url in seen:
                    continue
                if not clean_url.startswith("http"):
                    clean_url = "https://www.linkedin.com" + clean_url

                seen.add(clean_url)
                remote = "remote" in location.lower() or "remote" in title.lower()
                jobs.append(JobResult(
                    title=title,
                    company=company,
                    location=location,
                    department="",
                    apply_url=clean_url,
                    remote=remote,
                ))
                page_jobs += 1

            log.info(f"[linkedin] page {page_num + 1}: {page_jobs} jobs")
            if page_jobs == 0:
                break  # LinkedIn returned an empty page, no more results

        browser.close()

    return LinkedInScrapeResponse(jobs=jobs, total_found=len(jobs))


@app.post("/scrape/linkedin", response_model=LinkedInScrapeResponse)
def scrape_linkedin(req: LinkedInScrapeRequest):
    try:
        return _scrape_linkedin(req)
    except Exception as e:
        log.error(f"LinkedIn scrape error: {e}")
        return LinkedInScrapeResponse(jobs=[], total_found=0, error=str(e))

# ── Generic job page description fetcher ──────────────────────────────────────

class DescriptionRequest(BaseModel):
    url: str

class DescriptionResponse(BaseModel):
    text: str
    error: Optional[str] = None

# ── OSINT: HR/Recruiter Contact Finder ────────────────────────────────────────

class OSINTRequest(BaseModel):
    company: str
    domain: str = ""

class OSINTContact(BaseModel):
    name: str = ""
    title: str = ""
    email: str = ""
    linkedin: str = ""
    source: str = ""
    confidence: int = 0

class OSINTResponse(BaseModel):
    contacts: list[OSINTContact]
    sources_used: list[str]
    errors: list[str] = []


def _osint_emailfinder(domain: str) -> tuple[list[OSINTContact], str | None]:
    """Use emailfinder to harvest emails from Google/Bing/Baidu/Yandex dorks on a domain."""
    try:
        import warnings
        warnings.filterwarnings("ignore")
        from emailfinder.core import _get_emails
        emails = _get_emails(domain, proxy_dict=None)
        contacts = [
            OSINTContact(email=e.lower(), source="emailfinder", confidence=60)
            for e in (emails or []) if "@" in e and domain.split(".")[0] in e.lower()
        ]
        return contacts, None
    except ImportError:
        return [], "emailfinder not installed — run: pip install emailfinder"
    except Exception as e:
        return [], f"emailfinder error: {e}"


def _osint_crosslinked(company: str, domain: str) -> tuple[list[OSINTContact], str | None]:
    """
    Use CrossLinked to find employee names from Google/Bing LinkedIn dorks,
    then generate email patterns from those names.
    """
    import re

    try:
        from crosslinked.search import CrossLinked
    except ImportError:
        return [], "crosslinked not installed — run: pip install crosslinked"

    contacts: list[OSINTContact] = []
    try:
        # Search Google + Bing for LinkedIn profiles at this company
        for engine in ("google", "bing"):
            try:
                cl = CrossLinked(engine, company, timeout=30, conn_timeout=5)
                results = cl.search()
                for r in results:
                    name = r.get("name", "").strip()
                    title = r.get("title", "N/A").strip()
                    linkedin_url = r.get("url", "").strip()
                    if not name:
                        continue
                    # Generate email patterns from name + domain
                    name_parts = re.split(r"[\s,]+", name.lower())
                    emails_generated = []
                    if len(name_parts) >= 2 and domain:
                        first, last = name_parts[0], name_parts[-1]
                        emails_generated = [
                            f"{first}.{last}@{domain}",
                            f"{first[0]}{last}@{domain}",
                            f"{first}@{domain}",
                        ]
                    if emails_generated:
                        for email in emails_generated:
                            contacts.append(OSINTContact(
                                name=name, title=title if title != "N/A" else "",
                                email=email, linkedin=linkedin_url,
                                source="crosslinked", confidence=50,
                            ))
                    else:
                        contacts.append(OSINTContact(
                            name=name, title=title if title != "N/A" else "",
                            linkedin=linkedin_url,
                            source="crosslinked", confidence=45,
                        ))
            except Exception:
                pass
    except Exception as e:
        return [], f"crosslinked: {e}"

    return contacts, None


def _osint_theharvester(company: str, domain: str) -> tuple[list[OSINTContact], str | None]:
    """
    Run theHarvester against the domain using free sources (bing, yahoo, duckduckgo).
    Parse the JSON report it writes.
    """
    import subprocess, json, tempfile, os

    try:
        import theHarvester  # noqa: F401
    except ImportError:
        return [], "theHarvester not installed — run: pip install theHarvester"

    if not domain:
        return [], "theHarvester: domain required"

    with tempfile.TemporaryDirectory() as tmpdir:
        out_base = os.path.join(tmpdir, "harvest")
        try:
            subprocess.run(
                ["theHarvester",
                 "-d", domain,
                 "-b", "bing,yahoo,duckduckgo",
                 "-f", out_base,
                 "-l", "100"],
                capture_output=True, text=True, timeout=120,
                cwd=tmpdir,
            )
        except subprocess.TimeoutExpired:
            return [], "theHarvester timed out (120s)"
        except FileNotFoundError:
            return [], "theHarvester binary not found — run: pip install theHarvester"

        json_path = out_base + ".json"
        if not os.path.exists(json_path):
            return [], "theHarvester: no output produced"

        try:
            with open(json_path) as f:
                data = json.load(f)
        except Exception as e:
            return [], f"theHarvester: parse error {e}"

        contacts = []
        seen_emails: set[str] = set()
        for email in data.get("emails", []):
            e = str(email).lower().strip()
            if "@" not in e or e in seen_emails:
                continue
            seen_emails.add(e)
            contacts.append(OSINTContact(
                email=e, source="theharvester", confidence=65,
            ))
        return contacts, None


def _osint_linkedin_dork(company: str, domain: str) -> tuple[list[OSINTContact], str | None]:
    """
    Dork for LinkedIn HR/recruiter profiles using requests (no browser needed).
    Tries Bing and SearXNG public instances.
    """
    import re, time as _time, requests as _requests
    from bs4 import BeautifulSoup
    from urllib.parse import urlencode, quote_plus
    import random

    queries = [
        f'site:linkedin.com/in "{company}" recruiter OR "talent acquisition" OR "people operations"',
        f'site:linkedin.com/in "{company}" HR OR "human resources" OR hiring',
    ]

    contacts: list[OSINTContact] = []
    seen_urls: set[str] = set()

    agents = [
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
        "Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/121.0",
    ]

    def _parse_results(html: str, source_name: str) -> list[OSINTContact]:
        soup = BeautifulSoup(html, "html.parser")
        found = []
        for a in soup.find_all("a", href=True):
            href = a["href"]
            m = re.search(r"https?://(?:www\.)?linkedin\.com/in/([^/?&\s\"']+)", href)
            if not m:
                continue
            li_str = f"https://www.linkedin.com/in/{m.group(1)}"
            if li_str in seen_urls:
                continue
            seen_urls.add(li_str)

            # Try to get name/title from surrounding text
            raw_text = a.get_text(strip=True)
            parent_text = ""
            if a.parent:
                parent_text = a.parent.get_text(strip=True)

            name, title = "", ""
            # LinkedIn result title pattern: "FirstName LastName - Title at Company | LinkedIn"
            combined = raw_text or parent_text
            nm = re.match(r"^([A-Z][a-z]+(?: [A-Z][a-z]+)+)\s*[-–|]\s*(.+?)(?:\s*\||\s*at\s)", combined)
            if nm:
                name = nm.group(1).strip()
                title = nm.group(2).strip()
            else:
                name = combined.split("|")[0].split("-")[0].strip()

            email = ""
            if name and domain:
                parts = re.split(r"[\s,]+", name.lower())
                if len(parts) >= 2:
                    email = f"{parts[0]}.{parts[-1]}@{domain}"

            found.append(OSINTContact(
                name=name, title=title,
                email=email, linkedin=li_str,
                source=source_name, confidence=50,
            ))
        return found

    # Try Bing (with Playwright for JS rendering since requests gets redirected)
    try:
        from playwright.sync_api import sync_playwright
        with sync_playwright() as p:
            browser = p.chromium.launch(headless=True)
            ctx = browser.new_context(
                user_agent=random.choice(agents),
                locale="en-US",
                extra_http_headers={"Accept-Language": "en-US,en;q=0.9"},
            )
            page = ctx.new_page()
            # Accept Bing cookie consent if it appears
            for q in queries[:1]:  # Just one query to avoid rate limiting
                try:
                    url = "https://www.bing.com/search?" + urlencode({"q": q, "count": "20"})
                    page.goto(url, wait_until="domcontentloaded", timeout=15000)
                    _time.sleep(2)
                    # Click "Accept" if consent overlay appears
                    try:
                        btn = page.query_selector("#bnp_btn_accept, [id*=accept], button:has-text('Accept')")
                        if btn:
                            btn.click()
                            _time.sleep(1)
                    except Exception:
                        pass
                    html = page.content()
                    new_contacts = _parse_results(html, "linkedin_dork")
                    contacts.extend(new_contacts)
                except Exception as e:
                    log.debug(f"[osint] bing dork query failed: {e}")
            browser.close()
    except Exception:
        pass

    # Try requests-based Bing (mobile UA sometimes works without consent)
    if not contacts:
        try:
            for q in queries[:1]:
                url = "https://www.bing.com/search?" + urlencode({"q": q, "count": "20"})
                resp = _requests.get(url,
                    headers={
                        "User-Agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1",
                        "Accept": "text/html",
                        "Accept-Language": "en-US,en;q=0.9",
                    },
                    timeout=10, allow_redirects=True,
                )
                if resp.status_code == 200:
                    new_contacts = _parse_results(resp.text, "linkedin_dork")
                    contacts.extend(new_contacts)
        except Exception:
            pass

    return contacts, None


@app.post("/osint/contacts", response_model=OSINTResponse)
def osint_contacts(req: OSINTRequest):
    """
    Multi-source OSINT to find HR/recruiter contacts at a company.
    Runs all available free tools in sequence and merges results.
    Sources: emailfinder, crosslinked, theHarvester, google_dork (Playwright).
    """
    contacts: list[OSINTContact] = []
    sources_used: list[str] = []
    errors: list[str] = []
    seen: set[str] = set()

    def _merge(new_contacts: list[OSINTContact], source: str, err: str | None):
        if err:
            errors.append(err)
            return
        if not new_contacts:
            return
        sources_used.append(source)
        for c in new_contacts:
            key = c.email.lower() if c.email else c.linkedin
            if not key or key in seen:
                continue
            seen.add(key)
            contacts.append(c)

    domain = req.domain.strip()
    company = req.company.strip()

    # 1. emailfinder — fast domain-based Google/Bing dork for real emails
    if domain:
        cs, err = _osint_emailfinder(domain)
        _merge(cs, "emailfinder", err)

    # 2. CrossLinked — LinkedIn employee enumeration via search engine dorks
    cs, err = _osint_crosslinked(company, domain)
    _merge(cs, "crosslinked", err)

    # 3. theHarvester — multi-source email harvesting
    if domain:
        cs, err = _osint_theharvester(company, domain)
        _merge(cs, "theharvester", err)

    # 4. Playwright Google dork — LinkedIn profile + name/title extraction
    cs, err = _osint_linkedin_dork(company, domain)
    _merge(cs, "google_dork", err)

    # Sort by confidence desc
    contacts.sort(key=lambda c: c.confidence, reverse=True)

    return OSINTResponse(contacts=contacts, sources_used=sources_used, errors=errors)


@app.post("/description", response_model=DescriptionResponse)
def fetch_description(req: DescriptionRequest):
    try:
        from playwright.sync_api import sync_playwright
        from bs4 import BeautifulSoup
        import re

        with sync_playwright() as p:
            browser = p.chromium.launch(headless=True)
            ctx = browser.new_context(
                user_agent="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
                viewport={"width": 1280, "height": 800},
            )
            page = ctx.new_page()
            page.goto(req.url, wait_until="domcontentloaded", timeout=20000)
            if "linkedin.com" in req.url:
                try:
                    page.wait_for_selector(".description__text, #job-details, .jobs-description", timeout=8000)
                except Exception:
                    pass
                # Click "Show more" to expand full description
                for btn_sel in [
                    "button.show-more-less-html__button--more",
                    "button[aria-label*='Show more']",
                    ".jobs-description__footer-button",
                ]:
                    try:
                        btn = page.query_selector(btn_sel)
                        if btn:
                            btn.click()
                            page.wait_for_timeout(800)
                            break
                    except Exception:
                        pass
            html = page.content()
            browser.close()

        soup = BeautifulSoup(html, "html.parser")
        for tag in soup(["script", "style", "nav", "header", "footer", "aside"]):
            tag.decompose()

        text = ""
        if "linkedin.com" in req.url:
            for sel in ["#job-details", ".description__text", ".jobs-description-content", ".jobs-description"]:
                el = soup.select_one(sel)
                if el:
                    text = el.get_text(separator="\n", strip=True)
                    break

        if not text:
            candidates = soup.find_all(["article", "main", "section", "div"])
            if candidates:
                text = max((c.get_text(separator="\n", strip=True) for c in candidates), key=len, default="")

        import re as _re
        text = _re.sub(r"(?i)[ \t]*(Show more|Show less|Follow|Report this job|Save|Easy Apply|Apply Now|Apply)[ \t]*", " ", text)
        text = _re.sub(r"\n{3,}", "\n\n", text).strip()
        if len(text) < 50:
            return DescriptionResponse(text="", error="description too short or not found")
        return DescriptionResponse(text=text[:8000])

    except ImportError:
        return DescriptionResponse(text="", error="playwright not installed")
    except Exception as e:
        log.error(f"description fetch error: {e}")
        return DescriptionResponse(text="", error=str(e))
