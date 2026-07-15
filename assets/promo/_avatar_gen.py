import os, json, time, requests, pathlib
KEY=os.environ["KIE_KEY"].strip()
BASE="https://api.kie.ai/api/v1/gpt4o-image"
H={"Authorization":f"Bearer {KEY}","Content-Type":"application/json"}
HERE=pathlib.Path(__file__).resolve().parent
prompt=(
 "App icon / Telegram bot avatar, square 1:1, centered, no text at all. "
 "One large glossy tactile 3D shield in a vivid blue gradient (#2f6bff to #4aa8ff) with a bold letter 'V' cut into it, "
 "soft realistic shadows, smooth plastic-like highlights, floating look. "
 "Background: light and airy soft cream-to-white with a gentle light-blue tint and subtle abstract rounded shapes, "
 "a few tiny blue lightning bolts and sparkles around the shield. "
 "Bright cheerful clean premium fintech vibe, soft studio lighting, lots of light space. "
 "NOT a dark theme, NOT a full-blue background. No watermark, no people, no clutter, no letters other than the V on the shield."
)
body={"prompt":prompt,"size":"1:1","nVariants":1,"isEnhance":False,"enableFallback":True}
r=requests.post(f"{BASE}/generate",headers=H,data=json.dumps(body,ensure_ascii=False).encode(),timeout=60)
tid=(r.json().get("data") or {}).get("taskId")
print(">> avatar taskId=",tid,flush=True)
for i in range(1,61):
    time.sleep(10)
    d=requests.get(f"{BASE}/record-info",headers=H,params={"taskId":tid},timeout=60).json().get("data") or {}
    resp=d.get("response") or {}
    url=(resp.get("resultUrls") or resp.get("result_urls") or [None])[0]
    st=d.get("status")
    if url:
        img=requests.get(url,timeout=120).content
        (HERE/"_avatar.png").write_bytes(img)
        print(f">> saved _avatar.png ({len(img)} bytes)",flush=True); break
    if st in ("GENERATE_FAILED","FAILED","CREATE_TASK_FAILED"):
        print("FAIL",d); break
    print(f"  [{i}] {st}",flush=True)
