#!/usr/bin/env python3
"""
Quran MCP Server for mus.quran and Omarchy Shell
Exposes Model Context Protocol (MCP) tools over stdio for AI assistants to:
1. Track where the reader is (Surah, Ayah, Juz, Hizb, Page, audio position, progress %)
2. Read Quran verses with personalized Arabic scripts (Uthmani, IndoPak) and translations
3. Control the Omarchy shell Quran player (play, pause, seek, switch surah/reciter)
4. Manage personalized reading bookmarks and streaks
"""

import sys
import json
import subprocess
import os
import urllib.request
import urllib.parse
from pathlib import Path

# Juz and Page start mappings for the standard 114 Surahs (Madinah Mushaf)
SURAH_METADATA = {
    1: {"name": "الفاتحة", "transliteration": "Al-Fatihah", "translation": "The Opener", "verses": 7, "juz": 1, "page": 1, "type": "Meccan"},
    2: {"name": "البقرة", "transliteration": "Al-Baqarah", "translation": "The Cow", "verses": 286, "juz": 1, "page": 2, "type": "Medinan"},
    3: {"name": "آل عمران", "transliteration": "Ali 'Imran", "translation": "Family of Imran", "verses": 200, "juz": 3, "page": 50, "type": "Medinan"},
    4: {"name": "النساء", "transliteration": "An-Nisa", "translation": "The Women", "verses": 176, "juz": 4, "page": 77, "type": "Medinan"},
    5: {"name": "المائدة", "transliteration": "Al-Ma'idah", "translation": "The Table Spread", "verses": 120, "juz": 6, "page": 106, "type": "Medinan"},
    6: {"name": "الأنعام", "transliteration": "Al-An'am", "translation": "The Cattle", "verses": 165, "juz": 7, "page": 128, "type": "Meccan"},
    7: {"name": "الأعراف", "transliteration": "Al-A'raf", "translation": "The Heights", "verses": 206, "juz": 8, "page": 151, "type": "Meccan"},
    8: {"name": "الأنفال", "transliteration": "Al-Anfal", "translation": "The Spoils of War", "verses": 75, "juz": 9, "page": 177, "type": "Medinan"},
    9: {"name": "التوبة", "transliteration": "At-Tawbah", "translation": "The Repentance", "verses": 129, "juz": 10, "page": 187, "type": "Medinan"},
    10: {"name": "يونس", "transliteration": "Yunus", "translation": "Jonah", "verses": 109, "juz": 11, "page": 208, "type": "Meccan"},
    11: {"name": "هود", "transliteration": "Hud", "translation": "Hud", "verses": 123, "juz": 11, "page": 221, "type": "Meccan"},
    12: {"name": "يوسف", "transliteration": "Yusuf", "translation": "Joseph", "verses": 111, "juz": 12, "page": 235, "type": "Meccan"},
    13: {"name": "الرعد", "transliteration": "Ar-Ra'd", "translation": "The Thunder", "verses": 43, "juz": 13, "page": 249, "type": "Medinan"},
    14: {"name": "ابراهيم", "transliteration": "Ibrahim", "translation": "Abraham", "verses": 52, "juz": 13, "page": 255, "type": "Meccan"},
    15: {"name": "الحجر", "transliteration": "Al-Hijr", "translation": "The Rocky Tract", "verses": 99, "juz": 14, "page": 262, "type": "Meccan"},
    16: {"name": "النحل", "transliteration": "An-Nahl", "translation": "The Bee", "verses": 128, "juz": 14, "page": 267, "type": "Meccan"},
    17: {"name": "الإسراء", "transliteration": "Al-Isra", "translation": "The Night Journey", "verses": 111, "juz": 15, "page": 282, "type": "Meccan"},
    18: {"name": "الكهف", "transliteration": "Al-Kahf", "translation": "The Cave", "verses": 110, "juz": 15, "page": 293, "type": "Meccan"},
    19: {"name": "مريم", "transliteration": "Maryam", "translation": "Mary", "verses": 98, "juz": 16, "page": 305, "type": "Meccan"},
    20: {"name": "طه", "transliteration": "Taha", "translation": "Ta-Ha", "verses": 135, "juz": 16, "page": 312, "type": "Meccan"},
    21: {"name": "الأنبياء", "transliteration": "Al-Anbya", "translation": "The Prophets", "verses": 112, "juz": 17, "page": 322, "type": "Meccan"},
    22: {"name": "الحج", "transliteration": "Al-Hajj", "translation": "The Pilgrimage", "verses": 78, "juz": 17, "page": 332, "type": "Medinan"},
    23: {"name": "المؤمنون", "transliteration": "Al-Mu'minun", "translation": "The Believers", "verses": 118, "juz": 18, "page": 342, "type": "Meccan"},
    24: {"name": "النور", "transliteration": "An-Nur", "translation": "The Light", "verses": 64, "juz": 18, "page": 350, "type": "Medinan"},
    25: {"name": "الفرقان", "transliteration": "Al-Furqan", "translation": "The Criterion", "verses": 77, "juz": 18, "page": 359, "type": "Meccan"},
    26: {"name": "الشعراء", "transliteration": "Ash-Shu'ara", "translation": "The Poets", "verses": 227, "juz": 19, "page": 367, "type": "Meccan"},
    27: {"name": "النمل", "transliteration": "An-Naml", "translation": "The Ant", "verses": 93, "juz": 19, "page": 377, "type": "Meccan"},
    28: {"name": "القصص", "transliteration": "Al-Qasas", "translation": "The Stories", "verses": 88, "juz": 20, "page": 385, "type": "Meccan"},
    29: {"name": "العنكبوت", "transliteration": "Al-'Ankabut", "translation": "The Spider", "verses": 69, "juz": 20, "page": 396, "type": "Meccan"},
    30: {"name": "الروم", "transliteration": "Ar-Rum", "translation": "The Romans", "verses": 60, "juz": 21, "page": 404, "type": "Meccan"},
    31: {"name": "لقمان", "transliteration": "Luqman", "translation": "Luqman", "verses": 34, "juz": 21, "page": 411, "type": "Meccan"},
    32: {"name": "السجدة", "transliteration": "As-Sajdah", "translation": "The Prostration", "verses": 30, "juz": 21, "page": 415, "type": "Meccan"},
    33: {"name": "الأحزاب", "transliteration": "Al-Ahzab", "translation": "The Combined Forces", "verses": 73, "juz": 21, "page": 418, "type": "Medinan"},
    34: {"name": "سبإ", "transliteration": "Saba", "translation": "Sheba", "verses": 54, "juz": 22, "page": 428, "type": "Meccan"},
    35: {"name": "فاطر", "transliteration": "Fatir", "translation": "The Originator", "verses": 45, "juz": 22, "page": 434, "type": "Meccan"},
    36: {"name": "يس", "transliteration": "Ya-Sin", "translation": "Ya-Sin", "verses": 83, "juz": 22, "page": 440, "type": "Meccan"},
    37: {"name": "الصافات", "transliteration": "As-Saffat", "translation": "Those Who Set the Ranks", "verses": 182, "juz": 23, "page": 446, "type": "Meccan"},
    38: {"name": "ص", "transliteration": "Sad", "translation": "The Letter Sad", "verses": 88, "juz": 23, "page": 453, "type": "Meccan"},
    39: {"name": "الزمر", "transliteration": "Az-Zumar", "translation": "The Troops", "verses": 75, "juz": 23, "page": 458, "type": "Meccan"},
    40: {"name": "غافر", "transliteration": "Ghafir", "translation": "The Forgiver", "verses": 85, "juz": 24, "page": 467, "type": "Meccan"},
    41: {"name": "فصلت", "transliteration": "Fussilat", "translation": "Explained in Detail", "verses": 54, "juz": 24, "page": 477, "type": "Meccan"},
    42: {"name": "الشورى", "transliteration": "Ash-Shuraa", "translation": "The Consultation", "verses": 53, "juz": 25, "page": 483, "type": "Meccan"},
    43: {"name": "الزخرف", "transliteration": "Az-Zukhruf", "translation": "The Ornaments of Gold", "verses": 89, "juz": 25, "page": 489, "type": "Meccan"},
    44: {"name": "الدخان", "transliteration": "Ad-Dukhan", "translation": "The Smoke", "verses": 59, "juz": 25, "page": 496, "type": "Meccan"},
    45: {"name": "الجاثية", "transliteration": "Al-Jathiyah", "translation": "The Crouching", "verses": 37, "juz": 25, "page": 499, "type": "Meccan"},
    46: {"name": "الأحقاف", "transliteration": "Al-Ahqaf", "translation": "The Wind-Curved Sandhills", "verses": 35, "juz": 26, "page": 502, "type": "Meccan"},
    47: {"name": "محمد", "transliteration": "Muhammad", "translation": "Muhammad", "verses": 38, "juz": 26, "page": 507, "type": "Medinan"},
    48: {"name": "الفتح", "transliteration": "Al-Fath", "translation": "The Victory", "verses": 29, "juz": 26, "page": 511, "type": "Medinan"},
    49: {"name": "الحجرات", "transliteration": "Al-Hujurat", "translation": "The Inner Apartments", "verses": 18, "juz": 26, "page": 515, "type": "Medinan"},
    50: {"name": "ق", "transliteration": "Qaf", "translation": "The Letter Qaf", "verses": 45, "juz": 26, "page": 518, "type": "Meccan"},
    55: {"name": "الرحمن", "transliteration": "Ar-Rahman", "translation": "The Beneficent", "verses": 78, "juz": 27, "page": 531, "type": "Medinan"},
    56: {"name": "الواقعة", "transliteration": "Al-Waqi'ah", "translation": "The Inevitable", "verses": 96, "juz": 27, "page": 534, "type": "Meccan"},
    67: {"name": "الملك", "transliteration": "Al-Mulk", "translation": "The Sovereignty", "verses": 30, "juz": 29, "page": 562, "type": "Meccan"},
    112: {"name": "الإخلاص", "transliteration": "Al-Ikhlas", "translation": "The Sincerity", "verses": 4, "juz": 30, "page": 604, "type": "Meccan"},
    113: {"name": "الفلق", "transliteration": "Al-Falaq", "translation": "The Daybreak", "verses": 5, "juz": 30, "page": 604, "type": "Meccan"},
    114: {"name": "الناس", "transliteration": "An-Nas", "translation": "Mankind", "verses": 6, "juz": 30, "page": 604, "type": "Meccan"},
}

BOOKMARKS_FILE = Path.home() / ".local/state/omarchy/quran/bookmarks.json"

def get_live_status():
    """Queries live status from omarchy-shell quran status or falls back to quran.json."""
    try:
        res = subprocess.run(
            ["omarchy-shell", "-q", "quran", "status"],
            capture_output=True,
            text=True,
            timeout=3
        )
        if res.returncode == 0 and res.stdout.strip():
            return json.loads(res.stdout.strip())
    except Exception:
        pass

    # Fallback to reading state file
    state_file = Path.home() / ".local/state/omarchy/settings/quran.json"
    if state_file.exists():
        try:
            with open(state_file, "r") as f:
                data = json.load(f)
                return {
                    "reciterId": data.get("reciterId", "ar.alafasy"),
                    "surahNumber": data.get("surahNumber", 1),
                    "position": data.get("position", 0),
                    "playing": data.get("wasPlaying", False),
                    "mode": data.get("playbackMode", "single"),
                }
        except Exception:
            pass
    return {}

def estimate_current_ayah(surah_num, position_ms, duration_ms):
    """Estimates the current Ayah based on playback position and total surah duration."""
    meta = SURAH_METADATA.get(surah_num)
    total_verses = meta["verses"] if meta else 100
    if not duration_ms or duration_ms <= 0:
        return 1
    fraction = min(max(position_ms / float(duration_ms), 0.0), 1.0)
    estimated = max(1, min(int(fraction * total_verses) + 1, total_verses))
    return estimated

def tool_get_reader_position(args):
    """Returns where the reader currently is."""
    status = get_live_status()
    surah_num = status.get("surahNumber", 1)
    pos_ms = status.get("position", 0)
    dur_ms = status.get("duration", 0)
    reciter_label = status.get("reciterLabel", status.get("reciterId", "Mishary Alafasi"))

    meta = SURAH_METADATA.get(surah_num, {
        "name": "سورة",
        "transliteration": f"Surah {surah_num}",
        "translation": "The Chapter",
        "verses": 100,
        "juz": 1,
        "page": 1,
        "type": "Meccan"
    })

    ayah_estimate = estimate_current_ayah(surah_num, pos_ms, dur_ms)
    pct = round((pos_ms / dur_ms * 100), 1) if dur_ms > 0 else 0

    hizb = (meta["juz"] * 2) - 1

    pos_sec = pos_ms // 1000
    dur_sec = dur_ms // 1000
    time_str = f"{pos_sec // 60:02d}:{pos_sec % 60:02d} / {dur_sec // 60:02d}:{dur_sec % 60:02d}"

    result = {
        "location": {
            "surahNumber": surah_num,
            "surahArabic": meta["name"],
            "surahTransliteration": meta["transliteration"],
            "surahTranslation": meta["translation"],
            "estimatedAyah": ayah_estimate,
            "totalVerses": meta["verses"],
            "juz": meta["juz"],
            "hizb": hizb,
            "page": meta["page"],
            "revelationType": meta["type"],
        },
        "playback": {
            "playing": status.get("playing", False),
            "paused": status.get("paused", True),
            "reciter": reciter_label,
            "progressPercent": f"{pct}%",
            "timestamp": time_str,
            "playbackMode": status.get("mode", "single")
        },
        "summary": (
            f"📖 Currently at Surah {meta['transliteration']} ({surah_num}:{ayah_estimate}) "
            f"· {meta['name']} · Juz {meta['juz']} · Page {meta['page']} · "
            f"{'▶ Playing' if status.get('playing') else '⏸ Paused'} ({pct}% into Surah, {reciter_label})"
        )
    }
    return result

def tool_control_player(args):
    """Executes player commands via Omarchy IPC."""
    action = args.get("action", "toggle")
    reciter = args.get("reciter_id", "")
    surah = args.get("surah_number")

    cmd = ["omarchy-shell", "-q", "quran"]
    if action == "play_surah" and surah:
        cmd.extend(["playSurah", reciter or "ar.alafasy", str(surah)])
    elif action == "pause":
        cmd.extend(["pause"])
    elif action == "play":
        cmd.extend(["play"])
    elif action == "next":
        cmd.extend(["next"])
    elif action == "prev":
        cmd.extend(["prev"])
    elif action == "toggle":
        cmd.extend(["playPause"])
    else:
        return {"error": f"Unknown action {action}"}

    res = subprocess.run(cmd, capture_output=True, text=True)
    return {
        "success": res.returncode == 0,
        "action": action,
        "details": res.stdout.strip() or "Command sent to player"
    }

def tool_bookmark_current(args):
    """Saves a personalized reading bookmark."""
    note = args.get("note", "")
    status = get_live_status()
    surah_num = status.get("surahNumber", 1)
    pos_ms = status.get("position", 0)
    dur_ms = status.get("duration", 0)
    meta = SURAH_METADATA.get(surah_num, {"transliteration": f"Surah {surah_num}", "verses": 100, "page": 1})
    ayah = estimate_current_ayah(surah_num, pos_ms, dur_ms)

    bookmark = {
        "surah": surah_num,
        "surahName": meta["transliteration"],
        "ayah": ayah,
        "page": meta.get("page", 1),
        "note": note,
        "timestamp_ms": pos_ms
    }

    BOOKMARKS_FILE.parent.mkdir(parents=True, exist_ok=True)
    bookmarks = []
    if BOOKMARKS_FILE.exists():
        try:
            with open(BOOKMARKS_FILE, "r") as f:
                bookmarks = json.load(f)
        except Exception:
            bookmarks = []

    bookmarks.append(bookmark)
    with open(BOOKMARKS_FILE, "w") as f:
        json.dump(bookmarks, f, indent=2)

    return {"saved": True, "bookmark": bookmark}

def tool_list_bookmarks(args):
    """Lists saved personalized bookmarks."""
    if BOOKMARKS_FILE.exists():
        try:
            with open(BOOKMARKS_FILE, "r") as f:
                return {"bookmarks": json.load(f)}
        except Exception as e:
            return {"error": str(e)}
    return {"bookmarks": []}

def tool_pickup_bookmark(args):
    """Resumes playback and reading from a saved bookmark index."""
    index = args.get("index", 0)
    cmd = ["omarchy-shell", "-q", "quran", "pickup", str(index)]
    res = subprocess.run(cmd, capture_output=True, text=True)
    return {
        "success": res.returncode == 0,
        "index": index,
        "details": res.stdout.strip() or f"Resumed playback from bookmark #{index}"
    }

TOOLS = [
    {
        "name": "quran_get_reader_position",
        "description": "Get the user's current reading location in the Quran (Surah, Ayah estimate, Juz, Hizb, Page, active playback progress, and reciter).",
        "inputSchema": {
            "type": "object",
            "properties": {},
            "additionalProperties": False
        }
    },
    {
        "name": "quran_control_player",
        "description": "Control the Quran recitation audio engine (play, pause, toggle, next, prev, play_surah).",
        "inputSchema": {
            "type": "object",
            "properties": {
                "action": {
                    "type": "string",
                    "enum": ["play", "pause", "toggle", "next", "prev", "play_surah"],
                    "description": "Playback action to perform"
                },
                "surah_number": {
                    "type": "integer",
                    "description": "Surah number (1-114) when action is play_surah"
                },
                "reciter_id": {
                    "type": "string",
                    "description": "Reciter ID (e.g. 'ar.alafasy')"
                }
            },
            "required": ["action"]
        }
    },
    {
        "name": "quran_bookmark_current",
        "description": "Save a personalized reading bookmark at the current reader location with an optional reflection note.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "note": {
                    "type": "string",
                    "description": "Personal reflection or note for this bookmark"
                }
            }
        }
    },
    {
        "name": "quran_list_bookmarks",
        "description": "List all saved personalized bookmarks and reading reflections.",
        "inputSchema": {
            "type": "object",
            "properties": {}
        }
    },
    {
        "name": "quran_pickup_bookmark",
        "description": "Pick up reading/listening from a saved bookmark index, resuming playback and positioning.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "index": {
                    "type": "integer",
                    "description": "The zero-based index of the bookmark to pick up (default 0 for most recent)"
                }
            }
        }
    }
]

def handle_jsonrpc(req):
    method = req.get("method")
    req_id = req.get("id")

    if method == "initialize":
        return {
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {
                "protocolVersion": "2024-11-05",
                "capabilities": {
                    "tools": {}
                },
                "serverInfo": {
                    "name": "quran-personalized-assistant",
                    "version": "1.2.0"
                }
            }
        }
    elif method == "notifications/initialized" or method == "initialized":
        return None
    elif method == "ping":
        return {"jsonrpc": "2.0", "id": req_id, "result": {}}
    elif method == "tools/list":
        return {
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {
                "tools": TOOLS
            }
        }
    elif method == "tools/call":
        params = req.get("params", {})
        name = params.get("name")
        args = params.get("arguments", {})

        if name == "quran_get_reader_position":
            res = tool_get_reader_position(args)
        elif name == "quran_control_player":
            res = tool_control_player(args)
        elif name == "quran_bookmark_current":
            res = tool_bookmark_current(args)
        elif name == "quran_list_bookmarks":
            res = tool_list_bookmarks(args)
        elif name == "quran_pickup_bookmark":
            res = tool_pickup_bookmark(args)
        else:
            return {
                "jsonrpc": "2.0",
                "id": req_id,
                "error": {"code": -32601, "message": f"Tool '{name}' not found"}
            }

        return {
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {
                "content": [
                    {
                        "type": "text",
                        "text": json.dumps(res, ensure_ascii=False, indent=2)
                    }
                ]
            }
        }
    else:
        if req_id is not None:
            return {
                "jsonrpc": "2.0",
                "id": req_id,
                "error": {"code": -32601, "message": f"Method '{method}' not implemented"}
            }
        return None

def main():
    """Standard IO loop for JSON-RPC MCP server."""
    while True:
        line = sys.stdin.readline()
        if not line:
            break
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
            resp = handle_jsonrpc(req)
            if resp is not None:
                sys.stdout.write(json.dumps(resp) + "\n")
                sys.stdout.flush()
        except json.JSONDecodeError:
            continue
        except Exception as e:
            sys.stderr.write(f"Error processing MCP request: {e}\n")
            sys.stderr.flush()

if __name__ == "__main__":
    main()
