<script>
	import { DateTime } from 'luxon';
	export let date = new Date();
	export let tz = 'Europe/Oslo';

	let partDate = '';
	let partTime = '';

	$: {
		partDate = DateTime.fromJSDate(date).setZone(tz).toFormat('yyyy-MM-dd');
		partTime = DateTime.fromJSDate(date).setZone(tz).toFormat('HH:mm');
	}
	const updatePart = (partDate, partTime) => {
		date = DateTime.now()
			.setZone(tz)
			.set(DateTime.fromFormat(`${partDate} ${partTime}`, 'yyyy-MM-dd HH:mm').toObject())
			.toJSDate();
	};
</script>

<span>
	<input type="date" value={partDate} on:input={(e) => updatePart(e.target.value, partTime)} />
	<input type="time" value={partTime} on:input={(e) => updatePart(partDate, e.target.value)} />
</span>

<style>
	input {
		width: min-content;
	}
</style>
