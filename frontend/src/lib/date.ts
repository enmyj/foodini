export function todayStr(): string {
    const d = new Date();
    return [
        d.getFullYear(),
        String(d.getMonth() + 1).padStart(2, "0"),
        String(d.getDate()).padStart(2, "0"),
    ].join("-");
}

export function addDays(dateStr: string, n: number): string {
    const d = new Date(dateStr + "T12:00:00");
    d.setDate(d.getDate() + n);
    return d.toISOString().slice(0, 10);
}

export function getMonday(dateStr: string): string {
    const d = new Date(dateStr + "T12:00:00");
    const day = d.getDay();
    const diff = day === 0 ? -6 : 1 - day;
    d.setDate(d.getDate() + diff);
    return d.toISOString().slice(0, 10);
}

export function formatDateNav(dateStr: string): string {
    const today = todayStr();
    if (dateStr === today) return "Today";
    if (dateStr === addDays(today, -1)) return "Yesterday";
    const d = new Date(dateStr + "T12:00:00");
    return d.toLocaleDateString("en-US", {
        weekday: "short",
        month: "short",
        day: "numeric",
    });
}

export function formatTimeShort(hhmm: string | null | undefined): string {
    if (!hhmm) return "";
    const [hStr, mStr] = hhmm.split(":");
    if (!hStr || !mStr) return "";
    const h = Number(hStr);
    const m = Number(mStr);
    if (!Number.isFinite(h) || !Number.isFinite(m)) return "";
    const suffix = h >= 12 ? "pm" : "am";
    const h12 = h % 12 === 0 ? 12 : h % 12;
    const mPad = String(m).padStart(2, "0");
    return `${h12}:${mPad}${suffix}`;
}

export function formatWeekRange(start: string, end: string): string {
    const s = new Date(start + "T12:00:00");
    const e = new Date(end + "T12:00:00");
    const sm = s.toLocaleDateString("en-US", { month: "short" });
    const em = e.toLocaleDateString("en-US", { month: "short" });
    if (sm === em) return `${sm} ${s.getDate()}–${e.getDate()}`;
    return `${sm} ${s.getDate()} – ${em} ${e.getDate()}`;
}
