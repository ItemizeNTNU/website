import { DateTime } from "luxon";

/**
 * Nice string format for multiple data formats
 * @param {String|number|Date|DateTime} date 
 * @returns "I dag kl 17:15" or "tirsdag 1. februar kl 17:15" or "tirsdag 1. februar 2021 kl 17:15"
 */
export const smartFormat = (date) => {
	if (!date) {
		date = new Date();
	}
	if (['number', 'string'].includes(typeof date)) {
		date = new Date(date);
	}
	if (date instanceof Date) {
		date = DateTime.fromJSDate(date);
	}

	date = date.setZone('Europe/Oslo').setLocale('no');
	const now = DateTime.now().setZone('Europe/Oslo');
	let format = "'I dag kl' HH:mm";
	if (date.month != now.month || date.day != now.day) {
		format = "EEEE d. MMMM 'kl' HH:mm";
	}
	if (date.year != now.year) {
		format = "EEEE d. MMMM y 'kl' HH:mm";
	}
	return date.toFormat(format);
};