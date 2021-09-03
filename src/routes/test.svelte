<script>
	import TimePicker from '../components/TimePicker.svelte';

	let date;
</script>

<main>
	<h1>Test page</h1>
	<p>Select a date:</p>
	<TimePicker bind:date />
	<p>Date: {date}</p>
	<input type="time" name="time" id="time" />
</main>
