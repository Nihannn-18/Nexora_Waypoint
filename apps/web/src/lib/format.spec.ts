import {
  addDays,
  businessDate,
  formatCountdown,
  formatDay,
  formatInstantDay,
  formatTime,
  humanize,
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
