import {
  addDays,
  businessDate,
  formatCountdown,
  formatDay,
  formatInstantDay,
  formatTime,
  humanize,
  msUntilCutoff,
} from './format';

describe('format', () => {
  it('reads dates as the style guide says: "Sat 26 Sep"', () => {
    expect(formatDay('2026-09-26')).toBe('Sat 26 Sep');
    expect(formatDay('2026-10-04')).toBe('Sun 4 Oct');
  });

  it('formats instants in Asia/Colombo, not the machine timezone', () => {
    // 22:30 UTC on the 25th is 04:00 on the 26th in Colombo.
    const instant = '2026-09-25T22:30:00Z';
    expect(businessDate(new Date(instant))).toBe('2026-09-26');
    expect(formatInstantDay(instant)).toBe('Sat 26 Sep');
    expect(formatTime(instant)).toBe('04:00');
  });

  it('adds calendar days across a month boundary', () => {
    expect(addDays('2026-09-30', 1)).toBe('2026-10-01');
    expect(addDays('2026-10-01', -14)).toBe('2026-09-17');
  });

  it('formats a countdown and never goes negative', () => {
    expect(formatCountdown(42 * 60_000 + 15_000)).toBe('00:42:15');
    expect(formatCountdown(-5_000)).toBe('00:00:00');
  });

  it('humanizes wire enums', () => {
    expect(humanize('REAR_DOCK')).toBe('Rear dock');
    expect(humanize('IN_WORKSHOP')).toBe('In workshop');
  });
});

describe('msUntilCutoff (real-time 16:00 countdown)', () => {
  // Local wall-clock times on any day: the device's own time zone.
  const at = (h: number, m: number, sec = 0) => new Date(2026, 9, 4, h, m, sec);
  const MIN = 60_000;

  it('counts down to 16:00 from the current time', () => {
    expect(msUntilCutoff(at(14, 30))).toBe(90 * MIN);
    expect(msUntilCutoff(at(15, 45))).toBe(15 * MIN);
    expect(msUntilCutoff(at(15, 59))).toBe(1 * MIN);
  });

  it('keeps seconds exact', () => {
    expect(msUntilCutoff(at(15, 59, 30))).toBe(30_000);
    expect(formatCountdown(msUntilCutoff(at(14, 30)))).toBe('01:30:00');
  });

  it('is exactly zero at 16:00 and never negative after it', () => {
    expect(msUntilCutoff(at(16, 0))).toBe(0);
    expect(msUntilCutoff(at(16, 0, 1))).toBe(0);
    expect(msUntilCutoff(at(23, 59))).toBe(0);
  });
});
