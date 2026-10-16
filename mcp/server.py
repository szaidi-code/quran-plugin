#!/usr/bin/env python3
"""
Quran MCP Server for szaidi.quran and Omarchy Shell
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
import time
from datetime import datetime, timezone
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
    35: {"name": "فاطر", "transliteration": "Fatir", "translation": "Originator", "verses": 45, "juz": 22, "page": 434, "type": "Meccan"},
    36: {"name": "يس", "transliteration": "Ya-Sin", "translation": "Ya Sin", "verses": 83, "juz": 22, "page": 440, "type": "Meccan"},
    37: {"name": "الصافات", "transliteration": "As-Saffat", "translation": "Those who set the Ranks", "verses": 182, "juz": 23, "page": 446, "type": "Meccan"},
    38: {"name": "ص", "transliteration": "Sad", "translation": "The Letter \"Saad\"", "verses": 88, "juz": 23, "page": 453, "type": "Meccan"},
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
    49: {"name": "الحجرات", "transliteration": "Al-Hujurat", "translation": "The Rooms", "verses": 18, "juz": 26, "page": 515, "type": "Medinan"},
    50: {"name": "ق", "transliteration": "Qaf", "translation": "The Letter \"Qaf\"", "verses": 45, "juz": 26, "page": 518, "type": "Meccan"},
    51: {"name": "الذاريات", "transliteration": "Adh-Dhariyat", "translation": "The Winnowing Winds", "verses": 60, "juz": 26, "page": 520, "type": "Meccan"},
    52: {"name": "الطور", "transliteration": "At-Tur", "translation": "The Mount", "verses": 49, "juz": 27, "page": 523, "type": "Meccan"},
    53: {"name": "النجم", "transliteration": "An-Najm", "translation": "The Star", "verses": 62, "juz": 27, "page": 526, "type": "Meccan"},
    54: {"name": "القمر", "transliteration": "Al-Qamar", "translation": "The Moon", "verses": 55, "juz": 27, "page": 528, "type": "Meccan"},
    55: {"name": "الرحمن", "transliteration": "Ar-Rahman", "translation": "The Beneficent", "verses": 78, "juz": 27, "page": 531, "type": "Medinan"},
    56: {"name": "الواقعة", "transliteration": "Al-Waqi'ah", "translation": "The Inevitable", "verses": 96, "juz": 27, "page": 534, "type": "Meccan"},
    57: {"name": "الحديد", "transliteration": "Al-Hadid", "translation": "The Iron", "verses": 29, "juz": 27, "page": 537, "type": "Medinan"},
    58: {"name": "المجادلة", "transliteration": "Al-Mujadila", "translation": "The Pleading Woman", "verses": 22, "juz": 28, "page": 542, "type": "Medinan"},
    59: {"name": "الحشر", "transliteration": "Al-Hashr", "translation": "The Exile", "verses": 24, "juz": 28, "page": 545, "type": "Medinan"},
    60: {"name": "الممتحنة", "transliteration": "Al-Mumtahanah", "translation": "She that is to be examined", "verses": 13, "juz": 28, "page": 549, "type": "Medinan"},
    61: {"name": "الصف", "transliteration": "As-Saf", "translation": "The Ranks", "verses": 14, "juz": 28, "page": 551, "type": "Medinan"},
    62: {"name": "الجمعة", "transliteration": "Al-Jumu'ah", "translation": "The Congregation, Friday", "verses": 11, "juz": 28, "page": 553, "type": "Medinan"},
    63: {"name": "المنافقون", "transliteration": "Al-Munafiqun", "translation": "The Hypocrites", "verses": 11, "juz": 28, "page": 554, "type": "Medinan"},
    64: {"name": "التغابن", "transliteration": "At-Taghabun", "translation": "The Mutual Disillusion", "verses": 18, "juz": 28, "page": 556, "type": "Medinan"},
    65: {"name": "الطلاق", "transliteration": "At-Talaq", "translation": "The Divorce", "verses": 12, "juz": 28, "page": 558, "type": "Medinan"},
    66: {"name": "التحريم", "transliteration": "At-Tahrim", "translation": "The Prohibition", "verses": 12, "juz": 28, "page": 560, "type": "Medinan"},
    67: {"name": "الملك", "transliteration": "Al-Mulk", "translation": "The Sovereignty", "verses": 30, "juz": 29, "page": 562, "type": "Meccan"},
    68: {"name": "القلم", "transliteration": "Al-Qalam", "translation": "The Pen", "verses": 52, "juz": 29, "page": 564, "type": "Meccan"},
    69: {"name": "الحاقة", "transliteration": "Al-Haqqah", "translation": "The Reality", "verses": 52, "juz": 29, "page": 566, "type": "Meccan"},
    70: {"name": "المعارج", "transliteration": "Al-Ma'arij", "translation": "The Ascending Stairways", "verses": 44, "juz": 29, "page": 568, "type": "Meccan"},
    71: {"name": "نوح", "transliteration": "Nuh", "translation": "Noah", "verses": 28, "juz": 29, "page": 570, "type": "Meccan"},
    72: {"name": "الجن", "transliteration": "Al-Jinn", "translation": "The Jinn", "verses": 28, "juz": 29, "page": 572, "type": "Meccan"},
    73: {"name": "المزمل", "transliteration": "Al-Muzzammil", "translation": "The Enshrouded One", "verses": 20, "juz": 29, "page": 574, "type": "Meccan"},
    74: {"name": "المدثر", "transliteration": "Al-Muddaththir", "translation": "The Cloaked One", "verses": 56, "juz": 29, "page": 575, "type": "Meccan"},
    75: {"name": "القيامة", "transliteration": "Al-Qiyamah", "translation": "The Resurrection", "verses": 40, "juz": 29, "page": 577, "type": "Meccan"},
    76: {"name": "الانسان", "transliteration": "Al-Insan", "translation": "The Man", "verses": 31, "juz": 29, "page": 578, "type": "Medinan"},
    77: {"name": "المرسلات", "transliteration": "Al-Mursalat", "translation": "The Emissaries", "verses": 50, "juz": 29, "page": 580, "type": "Meccan"},
    78: {"name": "النبإ", "transliteration": "An-Naba", "translation": "The Tidings", "verses": 40, "juz": 30, "page": 582, "type": "Meccan"},
    79: {"name": "النازعات", "transliteration": "An-Nazi'at", "translation": "Those who drag forth", "verses": 46, "juz": 30, "page": 583, "type": "Meccan"},
    80: {"name": "عبس", "transliteration": "'Abasa", "translation": "He Frowned", "verses": 42, "juz": 30, "page": 585, "type": "Meccan"},
    81: {"name": "التكوير", "transliteration": "At-Takwir", "translation": "The Overthrowing", "verses": 29, "juz": 30, "page": 586, "type": "Meccan"},
    82: {"name": "الإنفطار", "transliteration": "Al-Infitar", "translation": "The Cleaving", "verses": 19, "juz": 30, "page": 587, "type": "Meccan"},
    83: {"name": "المطففين", "transliteration": "Al-Mutaffifin", "translation": "The Defrauding", "verses": 36, "juz": 30, "page": 587, "type": "Meccan"},
    84: {"name": "الإنشقاق", "transliteration": "Al-Inshiqaq", "translation": "The Sundering", "verses": 25, "juz": 30, "page": 589, "type": "Meccan"},
    85: {"name": "البروج", "transliteration": "Al-Buruj", "translation": "The Mansions of the Stars", "verses": 22, "juz": 30, "page": 590, "type": "Meccan"},
    86: {"name": "الطارق", "transliteration": "At-Tariq", "translation": "The Nightcommer", "verses": 17, "juz": 30, "page": 591, "type": "Meccan"},
    87: {"name": "الأعلى", "transliteration": "Al-A'la", "translation": "The Most High", "verses": 19, "juz": 30, "page": 591, "type": "Meccan"},
    88: {"name": "الغاشية", "transliteration": "Al-Ghashiyah", "translation": "The Overwhelming", "verses": 26, "juz": 30, "page": 592, "type": "Meccan"},
    89: {"name": "الفجر", "transliteration": "Al-Fajr", "translation": "The Dawn", "verses": 30, "juz": 30, "page": 593, "type": "Meccan"},
    90: {"name": "البلد", "transliteration": "Al-Balad", "translation": "The City", "verses": 20, "juz": 30, "page": 594, "type": "Meccan"},
    91: {"name": "الشمس", "transliteration": "Ash-Shams", "translation": "The Sun", "verses": 15, "juz": 30, "page": 595, "type": "Meccan"},
    92: {"name": "الليل", "transliteration": "Al-Layl", "translation": "The Night", "verses": 21, "juz": 30, "page": 595, "type": "Meccan"},
    93: {"name": "الضحى", "transliteration": "Ad-Duhaa", "translation": "The Morning Hours", "verses": 11, "juz": 30, "page": 596, "type": "Meccan"},
    94: {"name": "الشرح", "transliteration": "Ash-Sharh", "translation": "The Relief", "verses": 8, "juz": 30, "page": 596, "type": "Meccan"},
    95: {"name": "التين", "transliteration": "At-Tin", "translation": "The Fig", "verses": 8, "juz": 30, "page": 597, "type": "Meccan"},
    96: {"name": "العلق", "transliteration": "Al-'Alaq", "translation": "The Clot", "verses": 19, "juz": 30, "page": 597, "type": "Meccan"},
    97: {"name": "القدر", "transliteration": "Al-Qadr", "translation": "The Power", "verses": 5, "juz": 30, "page": 598, "type": "Meccan"},
    98: {"name": "البينة", "transliteration": "Al-Bayyinah", "translation": "The Clear Proof", "verses": 8, "juz": 30, "page": 598, "type": "Medinan"},
    99: {"name": "الزلزلة", "transliteration": "Az-Zalzalah", "translation": "The Earthquake", "verses": 8, "juz": 30, "page": 599, "type": "Medinan"},
    100: {"name": "العاديات", "transliteration": "Al-'Adiyat", "translation": "The Courser", "verses": 11, "juz": 30, "page": 599, "type": "Meccan"},
    101: {"name": "القارعة", "transliteration": "Al-Qari'ah", "translation": "The Calamity", "verses": 11, "juz": 30, "page": 600, "type": "Meccan"},
    102: {"name": "التكاثر", "transliteration": "At-Takathur", "translation": "The Rivalry in world increase", "verses": 8, "juz": 30, "page": 600, "type": "Meccan"},
    103: {"name": "العصر", "transliteration": "Al-'Asr", "translation": "The Declining Day", "verses": 3, "juz": 30, "page": 601, "type": "Meccan"},
    104: {"name": "الهمزة", "transliteration": "Al-Humazah", "translation": "The Traducer", "verses": 9, "juz": 30, "page": 601, "type": "Meccan"},
    105: {"name": "الفيل", "transliteration": "Al-Fil", "translation": "The Elephant", "verses": 5, "juz": 30, "page": 601, "type": "Meccan"},
    106: {"name": "قريش", "transliteration": "Quraysh", "translation": "Quraysh", "verses": 4, "juz": 30, "page": 602, "type": "Meccan"},
    107: {"name": "الماعون", "transliteration": "Al-Ma'un", "translation": "The Small kindnesses", "verses": 7, "juz": 30, "page": 602, "type": "Meccan"},
    108: {"name": "الكوثر", "transliteration": "Al-Kawthar", "translation": "The Abundance", "verses": 3, "juz": 30, "page": 602, "type": "Meccan"},
    109: {"name": "الكافرون", "transliteration": "Al-Kafirun", "translation": "The Disbelievers", "verses": 6, "juz": 30, "page": 603, "type": "Meccan"},
    110: {"name": "النصر", "transliteration": "An-Nasr", "translation": "The Divine Support", "verses": 3, "juz": 30, "page": 603, "type": "Medinan"},
    111: {"name": "المسد", "transliteration": "Al-Masad", "translation": "The Palm Fiber", "verses": 5, "juz": 30, "page": 603, "type": "Meccan"},
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
    reciter_id = status.get("reciterId", "ar.alafasy")
    reciter_label = status.get("reciterLabel", reciter_id)
    meta = SURAH_METADATA.get(surah_num, {
        "name": "",
        "transliteration": f"Surah {surah_num}",
        "verses": 100,
        "juz": 1,
        "page": 1,
    })
    ayah = estimate_current_ayah(surah_num, pos_ms, dur_ms)

    bookmark = {
        "id": str(int(time.time() * 1000)),
        "surah": surah_num,
        "surahName": meta["transliteration"],
        "surahArabic": meta.get("name", ""),
        "ayah": ayah,
        "totalVerses": meta.get("verses", 100),
        "juz": meta.get("juz", 1),
        "page": meta.get("page", 1),
        "reciterId": reciter_id,
        "reciter": reciter_label,
        "timestamp_ms": pos_ms,
        "duration_ms": dur_ms,
        "note": note or f"Ayah {ayah}",
        "createdAt": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
    }

    BOOKMARKS_FILE.parent.mkdir(parents=True, exist_ok=True)
    bookmarks = []
    if BOOKMARKS_FILE.exists():
        try:
            with open(BOOKMARKS_FILE, "r") as f:
                bookmarks = json.load(f)
        except Exception:
            bookmarks = []

    bookmarks.insert(0, bookmark)
    tmp_file = BOOKMARKS_FILE.with_suffix(".json.tmp")
    with open(tmp_file, "w") as f:
        json.dump(bookmarks, f, indent=2)
    os.replace(tmp_file, BOOKMARKS_FILE)

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
