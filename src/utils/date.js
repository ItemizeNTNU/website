const months = { 0: 'Januar', 1: 'Februar', 2: 'Mars', 3: 'April', 4: 'Mai', 5: 'Juni', 6: 'Juli', 7: 'August', 8: 'September', 9: ' Oktober', 10: 'November', 11: 'Desember' };

export const nicePrintDate = (date) => {
	let dateString = date.getDate() + '. ';
	dateString += months[date.getMonth()] + ' ';
	dateString += date.getFullYear();
	return dateString;
};

export default { nicePrintDate };
