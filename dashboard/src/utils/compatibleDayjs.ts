import dayjs from 'dayjs';
import customParseFormat from 'dayjs/plugin/customParseFormat';
import timezone from 'dayjs/plugin/timezone';
import utc from 'dayjs/plugin/utc';

dayjs.extend(customParseFormat);
dayjs.extend(utc);
dayjs.extend(timezone);

const FORMAT = 'YYYY-MM-DD HH:mm:ss ZZ';

export default function compatibleDayjs(date: string) {
  return dayjs(date, FORMAT).tz();
}
