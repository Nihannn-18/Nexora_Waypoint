import {
  assessWindow,
  calculateTripTime,
  checkTimeBudget,
  clockToMinutes,
  estimateFuelLitres,
  isWithinWeeklyFuelQuota,
  minutesToClock,
} from './trip-time';
import {
  FRESH_TIME_BUDGET_MIN,
  STYLE_TECH_TIME_BUDGET_MIN,
} from './constraints';

/**
 * These are the shared test vectors. The Go TripTimeCalculator must produce
 * identical numbers for the same inputs — apps/api/internal/planning has the
 * mirrored table-driven test. If you change a number here, change it there.
 */
describe('calculateTripTime — official formula', () => {
  it('reproduces the worked example from the Challenge Booklet', () => {
    // Three-order Fresh trip to Gampaha: outbound 37, two inter-stop legs of 9,
    // handling 15 + 15 + 16. The brief states the total is 101 minutes.
    const result = calculateTripTime({
      outboundFreeflowMin: 37,
      interStopFreeflowMin: 9,
      serviceAllowancesMin: [15, 15, 16],
    });

    expect(result.outboundMin).toBe(37);
    expect(result.interStopMin).toBe(18);
    expect(result.handlingMin).toBe(46);
    expect(result.totalTripMin).toBe(101);
  });

  it('counts zero inter-stop journeys for a one-order trip', () => {
    const result = calculateTripTime({
      outboundFreeflowMin: 37,
      interStopFreeflowMin: 9,
      serviceAllowancesMin: [15],
    });

    expect(result.interStopMin).toBe(0);
    expect(result.totalTripMin).toBe(52);
  });

  it('counts two inter-stop journeys for a three-order trip', () => {
    const result = calculateTripTime({
      outboundFreeflowMin: 20,
      interStopFreeflowMin: 10,
      serviceAllowancesMin: [5, 5, 5],
    });

    expect(result.interStopMin).toBe(20);
  });

  it('charges nothing for a trip with no orders', () => {
    const result = calculateTripTime({
      outboundFreeflowMin: 37,
      interStopFreeflowMin: 9,
      serviceAllowancesMin: [],
    });

    expect(result.totalTripMin).toBe(0);
    expect(result.outboundMin).toBe(0);
  });
});

describe('checkTimeBudget', () => {
  const fresh = (used: number) => ({
    freshMinutesUsed: used,
    styleTechMinutesUsed: 0,
  });
  const styleTech = (used: number) => ({
    freshMinutesUsed: 0,
    styleTechMinutesUsed: used,
  });

  it('accepts a Fresh day totalling exactly 270 minutes', () => {
    const check = checkTimeBudget('FRESH', 70, fresh(200));
    expect(check.fits).toBe(true);
    expect(check.budgetMin).toBe(FRESH_TIME_BUDGET_MIN);
    expect(check.overByMin).toBe(0);
  });

  it('rejects a Fresh day totalling 271 minutes', () => {
    const check = checkTimeBudget('FRESH', 71, fresh(200));
    expect(check.fits).toBe(false);
    expect(check.overByMin).toBe(1);
  });

  it('accepts Style and Tech totalling exactly 480 minutes', () => {
    expect(checkTimeBudget('STYLE', 80, styleTech(400)).fits).toBe(true);
    expect(checkTimeBudget('TECH', 80, styleTech(400)).budgetMin).toBe(
      STYLE_TECH_TIME_BUDGET_MIN,
    );
  });

  it('rejects Style and Tech totalling 481 minutes', () => {
    expect(checkTimeBudget('TECH', 81, styleTech(400)).fits).toBe(false);
  });

  it('draws Style and Tech from one shared pool, not two', () => {
    // 300 minutes of Style already used; a 200-minute Tech trip must not fit.
    const check = checkTimeBudget('TECH', 200, styleTech(300));
    expect(check.fits).toBe(false);
    expect(check.usedMin).toBe(300);
  });

  it('keeps the Fresh pool independent of the Style/Tech pool', () => {
    const check = checkTimeBudget('FRESH', 200, {
      freshMinutesUsed: 0,
      styleTechMinutesUsed: 470,
    });
    expect(check.fits).toBe(true);
  });
});

describe('assessWindow', () => {
  it('waits when the vehicle arrives before the window opens', () => {
    const result = assessWindow('05:40', '06:00', '08:00');
    expect(result.waitingMin).toBe(20);
    expect(result.serviceStart).toBe('06:00');
    expect(result.late).toBe(false);
  });

  it('starts immediately when arrival is inside the window', () => {
    const result = assessWindow('07:15', '06:00', '08:00');
    expect(result.waitingMin).toBe(0);
    expect(result.serviceStart).toBe('07:15');
    expect(result.late).toBe(false);
  });

  it('is not late at exactly the closing minute', () => {
    expect(assessWindow('08:00', '06:00', '08:00').late).toBe(false);
  });

  it('is late one minute after the window closes', () => {
    const result = assessWindow('08:01', '06:00', '08:00');
    expect(result.late).toBe(true);
    expect(result.lateByMin).toBe(1);
  });

  it('rejects a malformed clock time rather than guessing', () => {
    expect(() => clockToMinutes('not-a-time')).toThrow(/Invalid clock time/);
  });
});

describe('clock helpers', () => {
  it('round-trips a time', () => {
    expect(minutesToClock(clockToMinutes('07:34'))).toBe('07:34');
  });

  it('pads single-digit hours and minutes', () => {
    expect(minutesToClock(5)).toBe('00:05');
  });
});

describe('fuel', () => {
  it('estimates litres from distance and efficiency', () => {
    expect(estimateFuelLitres(120, 8)).toBe(15);
  });

  it('accepts usage exactly at the weekly quota', () => {
    expect(isWithinWeeklyFuelQuota(90, 10, 100)).toBe(true);
  });

  it('rejects usage one litre over the weekly quota', () => {
    expect(isWithinWeeklyFuelQuota(90, 11, 100)).toBe(false);
  });

  it('refuses a nonsensical efficiency instead of returning Infinity', () => {
    expect(() => estimateFuelLitres(100, 0)).toThrow();
  });
});
