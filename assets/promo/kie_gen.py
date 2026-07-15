# -*- coding: utf-8 -*-
"""
Генератор 16 карточек VexaVPN через KIE (4o-image), text-to-image.
Светлый сине-голубой SaaS-стиль (как референс-семпл, но синий), русский текст ВСТРОЕН моделью.
Python решает проблему кодировки кириллицы, на которой спотыкался PowerShell 5.1.

Запуск:
  set KIE_KEY=xxx  (или передаётся из окружения)
  python kie_gen.py            # создать задачи + скачать всё
  python kie_gen.py create     # только создать задачи
  python kie_gen.py fetch       # только опросить + скачать по сохранённым taskId
"""
import os, sys, json, time, pathlib

import requests

KEY = os.environ.get("KIE_KEY", "").strip()
if not KEY:
    sys.exit("нет KIE_KEY в окружении")

BASE = "https://api.kie.ai/api/v1/gpt4o-image"
HEADERS = {"Authorization": f"Bearer {KEY}", "Content-Type": "application/json"}
HERE = pathlib.Path(__file__).resolve().parent
OUT = HERE / "cards"
OUT.mkdir(parents=True, exist_ok=True)
TASKS_FILE = HERE / "_py_tasks.json"

SIZE = "1:1"

# Единый стиль для всех карточек. Светлый фон, синяя палитра, глянцевая 3D-иконка,
# крупный жирный русский заголовок сверху, плашка-пилюля @vexa_vpn_bot снизу.
STYLE = (
    "Playful premium SaaS promo card, square 1:1, in the exact style of a modern app store feature graphic. "
    "Background: light and airy, soft cream-to-white with a gentle light-blue tint, decorated with subtle abstract "
    "rounded geometric shapes, soft blurred blobs and a few thin decorative lines and dots. "
    "Centered hero: one large glossy tactile 3D icon rendered in a vivid blue gradient (#2f6bff to #4aa8ff), "
    "with soft realistic shadows, smooth plastic-like highlights and a floating look. "
    "Playful accents floating around the icon: small blue lightning bolts, sparkles, dots and tiny geometric bits. "
    "Typography: a bold heading in Russian at the top in dark navy (#0b1f4d), large, rounded, friendly sans-serif, very easy to read. "
    "A small rounded pill badge near the bottom containing the handle '@vexa_vpn_bot' in blue. "
    "Bright, cheerful, clean, expensive fintech vibe, lots of light negative space, soft studio lighting. "
    "NOT a dark theme, NOT a full-blue background — the background stays light. No watermark, no realistic people, no clutter."
)

# name | русский заголовок (встраивается в картинку) | центральная 3D-иконка
SCREENS = [
    ("menu_main",      "VexaVPN",             "a glossy blue 3D shield with a subtle keyhole in the center"),
    ("subscriber",     "VPN активен",         "a glossy blue 3D shield with a bright white checkmark and a small glowing dot"),
    ("buy",            "Купить VPN",          "a glossy blue 3D shield merged with an upward rocket"),
    ("trial",          "7 дней бесплатно",    "a glossy blue 3D gift box with a glowing ribbon"),
    ("devices",        "Мои устройства",      "a glossy blue 3D phone, laptop and tablet linked by thin glowing lines"),
    ("settings",       "Мой тариф",           "a glossy blue 3D gear with slider controls"),
    ("promo",          "Промокод",            "a glossy blue 3D discount ticket tag with a percent sign"),
    ("payment",        "Оплата",              "a glossy blue 3D bank card with a small glowing lock"),
    ("activating",     "Активируем VPN",      "a glossy blue 3D circular loading spinner ring, mid-motion"),
    ("success",        "Готово",              "a big glossy blue 3D checkmark inside a glowing ring with subtle sparks"),
    ("help",           "Помощь",              "a glossy blue 3D question mark inside a rounded speech bubble"),
    ("invite",         "Пригласи друга",      "two glossy blue 3D person silhouettes with a glowing plus between them"),
    ("expired",        "Подписка истекла",    "a dimmed cracked blue 3D shield with a faded glow"),
    ("expiry",         "Подписка кончается",  "a glossy blue 3D hourglass with a thin glowing ring"),
    ("device_new",     "Новое устройство",    "a glossy blue 3D phone with a small glowing plus badge"),
    ("device_blocked", "Лимит устройств",     "a glossy blue 3D phone with a small lock badge, dimmed"),
]


def build_prompt(title, icon):
    return (
        f"{STYLE} "
        f"Central 3D icon: {icon}. "
        f'The heading text at the top must read exactly, in Russian: "{title}". '
        f"Render this Russian text crisply and correctly, no other text except the heading and the @vexa_vpn_bot pill."
    )


def create_all(screens=None):
    tasks = {}
    for name, title, icon in (screens if screens is not None else SCREENS):
        body = {
            "prompt": build_prompt(title, icon),
            "size": SIZE,
            "nVariants": 1,
            "isEnhance": False,
            "enableFallback": True,
        }
        try:
            r = requests.post(f"{BASE}/generate", headers=HEADERS,
                              data=json.dumps(body, ensure_ascii=False).encode("utf-8"), timeout=60)
            j = r.json()
            tid = (j.get("data") or {}).get("taskId")
            if tid:
                tasks[name] = tid
                print(f"created {name:15} {tid}", flush=True)
            else:
                print(f"NO TASKID {name}: {j}", flush=True)
        except Exception as e:
            print(f"ERR {name}: {e}", flush=True)
        time.sleep(1.5)
    TASKS_FILE.write_text(json.dumps(tasks, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"=== saved {len(tasks)} tasks -> {TASKS_FILE} ===", flush=True)
    return tasks


def result_url(data):
    resp = data.get("response") or {}
    for k in ("resultUrls", "result_urls"):
        v = resp.get(k)
        if v:
            return v[0]
    return None


def fetch_all():
    tasks = json.loads(TASKS_FILE.read_text(encoding="utf-8"))
    pending = dict(tasks)
    done = {}
    for attempt in range(1, 51):
        if not pending:
            break
        for name, tid in list(pending.items()):
            try:
                r = requests.get(f"{BASE}/record-info", headers=HEADERS,
                                 params={"taskId": tid}, timeout=60)
                data = r.json().get("data") or {}
                url = result_url(data)
                st = data.get("status")
                if url:
                    img = requests.get(url, timeout=120).content
                    (OUT / f"{name}.png").write_bytes(img)
                    print(f"[{attempt}] OK   {name:15} {len(img)} bytes", flush=True)
                    done[name] = url
                    del pending[name]
                elif st in ("GENERATE_FAILED", "FAILED", "CREATE_TASK_FAILED"):
                    print(f"[{attempt}] FAIL {name}: {r.json().get('msg')}", flush=True)
                    del pending[name]
                else:
                    print(f"[{attempt}] .... {name:15} {st}", flush=True)
            except Exception as e:
                print(f"[{attempt}] ERR {name}: {e}", flush=True)
        if pending:
            time.sleep(12)
    print(f"=== fetch done: {len(done)}/{len(tasks)} ===", flush=True)


def _gen_one(name, out_path):
    """Синхронно сгенерировать ОДНУ карточку по имени экрана в out_path."""
    scr = next((s for s in SCREENS if s[0] == name), None)
    if not scr:
        sys.exit(f"нет экрана {name} в SCREENS")
    _, title, icon = scr
    body = {
        "prompt": build_prompt(title, icon),
        "size": SIZE,
        "nVariants": 1,
        "isEnhance": False,
        "enableFallback": True,
    }
    r = requests.post(f"{BASE}/generate", headers=HEADERS,
                      data=json.dumps(body, ensure_ascii=False).encode("utf-8"), timeout=60)
    tid = (r.json().get("data") or {}).get("taskId")
    if not tid:
        sys.exit(f"нет taskId: {r.json()}")
    print(f">> {name} taskId={tid}", flush=True)
    for attempt in range(1, 61):
        time.sleep(10)
        r = requests.get(f"{BASE}/record-info", headers=HEADERS,
                         params={"taskId": tid}, timeout=60)
        data = r.json().get("data") or {}
        url = result_url(data)
        st = data.get("status")
        if url:
            img = requests.get(url, timeout=120).content
            out_path.write_bytes(img)
            print(f">> saved {out_path} ({len(img)} bytes)", flush=True)
            return
        if st in ("GENERATE_FAILED", "FAILED", "CREATE_TASK_FAILED"):
            sys.exit(f"FAIL: {r.json().get('msg')}")
        print(f"  [{attempt}] {st}", flush=True)
    sys.exit("timeout")


def test_one(name="buy"):
    """Прогнать ОДНУ карточку в _test_<name>.png — проверить стиль перед пачкой."""
    _gen_one(name, HERE / f"_test_{name}.png")


def one(name):
    """Перегенерировать ОДИН экран сразу в боевую папку cards/<name>.png."""
    _gen_one(name, OUT / f"{name}.png")


if __name__ == "__main__":
    mode = sys.argv[1] if len(sys.argv) > 1 else "all"
    if mode == "test":
        test_one(sys.argv[2] if len(sys.argv) > 2 else "buy")
        sys.exit(0)
    if mode == "one":
        if len(sys.argv) < 3:
            sys.exit("укажи имя экрана: python kie_gen.py one subscriber")
        one(sys.argv[2])
        sys.exit(0)
    if mode in ("all", "create"):
        create_all()
    if mode in ("all", "fetch"):
        if mode == "all":
            time.sleep(30)
        fetch_all()
