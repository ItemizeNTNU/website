<script>
	import FaArrowDown from 'svelte-icons/fa/FaArrowDown.svelte';
	import FaToggleOff from 'svelte-icons/fa/FaToggleOff.svelte';
	import FaToggleOn from 'svelte-icons/fa/FaToggleOn.svelte';
	import FaPowerOff from 'svelte-icons/fa/FaPowerOff.svelte';
	import Button from '../components/Button.svelte';
	import ToggleIcon from '../components/ToggleIcon.svelte';
	import TimePicker from '../components/TimePicker.svelte';
	import Modal from '../components/Modal.svelte';
	import { smartFormat } from '../utils/time';

	import { toasts } from 'svelte-toasts';
	import { confirm, confirm_strong, prompt, alert } from '../utils/prompt';

	let rotate = 180;

	let toggle_value;
	const onToggle = (value) => value && toasts.info('Toggle is now on');

	let sticky;
	let stickyCounter;
	const toggleSticky = () => {
		if (stickyCounter >= 5) {
			sticky.remove();
			stickyCounter = 0;
		} else if (stickyCounter > 0) {
			stickyCounter += 1;
			sticky.update({ description: `Hi there x${stickyCounter}` });
		} else {
			stickyCounter = 1;
			sticky = toasts.info('Sticky', 'Hi there x1', { duration: 0 });
		}
	};

	let date;

	let modalClean;
	let modalFull;

	let modalDouble1;
	let modalDouble2;

	const modalAction = (error) => () => {
		const action = error ? toasts.error : toasts.success;
		action(`You ${error ? 'declined' : 'accepted'}.`);
	};
</script>

<main>
	<h1>Buttons:</h1>
	<p>
		Rotate Button:
		<Button submit={() => (rotate = (rotate + 180) % 360)} {rotate} icon={FaArrowDown} />
		<Button submit={() => (rotate = (rotate + 180) % 360)} {rotate} icon={FaArrowDown}>Rotate Button</Button>
		<Button submit={() => toasts.info('Hi there')}>Alert Button</Button>
	</p>

	<p>
		<Button fill big>Big Fill Button</Button>
	</p>
	<p class="row">
		<Button>Normal Button In Row 1</Button>
		<Button>Normal Button In Row 2</Button>
		<Button>Normal Button In Row 3</Button>
	</p>
	<p class="row">
		<Button>None</Button>
		<Button info>Info</Button>
		<Button success>Success</Button>
		<Button warning>Warning</Button>
		<Button error>Error</Button>
	</p>
	<p>
		<ToggleIcon rotate={180} />
		<ToggleIcon iconOn={FaToggleOn} iconOff={FaToggleOff} />
		<ToggleIcon iconOff={FaPowerOff} colorOff="#888" colorOn="#fff" bind:value={toggle_value} {onToggle} />
	</p>

	<hr />
	<h1>Alerts:</h1>
	<p class="row">
		<Button info submit={() => toasts.info('Hi there')}>Info</Button>
		<Button info submit={() => toasts.info('hi', 'Hi there')}>Info Tittle</Button>
		<Button success submit={() => toasts.success('hi', 'Hi there')}>Success Tittle</Button>
		<Button warning submit={() => toasts.warning('hi', 'Hi there')}>Warning Tittle</Button>
		<Button error submit={() => toasts.error('hi', 'Hi there')}>Error Tittle</Button>
	</p>
	<p class="row">
		<Button info submit={toggleSticky}>Info Sticky {stickyCounter ? (stickyCounter == 10 ? 'Remove' : 'Update') : 'Add'}</Button>
	</p>

	<hr />
	<h1>Inputs:</h1>
	<p>
		<TimePicker bind:date />
		{smartFormat(date)}
	</p>

	<hr />
	<h1>Modals:</h1>
	<div class="row">
		<Button info submit={() => (modalClean = true)}>Show Clean Modal</Button>
		<Modal bind:show={modalClean} onClose={modalAction(true)}>This is an example Modal</Modal>

		<Button info submit={() => (modalFull = true)}>Show Full Modal</Button>
		<Modal bind:show={modalFull} title="Example title" acceptButton="Accept" onAccept={modalAction(false)} closeButton="Close" onClose={modalAction(true)}>
			This is an example Modal
		</Modal>

		<Button info submit={() => (modalDouble1 = true)}>Show Double Modal</Button>
		<Modal bind:show={modalDouble1}>
			<Button info submit={() => (modalDouble2 = true)}>And another one!</Button>
			<Modal bind:show={modalDouble2} closeButton>Hi there</Modal>
		</Modal>
	</div>

	<hr />
	<h1>Prompts:</h1>
	<div class="row">
		<Button
			info
			submit={async () => {
				if (await confirm('Are you sure you to to do X?')) {
					toasts.success('Confirmed successfully');
				} else {
					toasts.error('Cancelled');
				}
			}}>Confirm</Button>

		<Button
			info
			submit={async () => {
				if (await confirm_strong('Are you sure you want to do X?', 'i_confirm')) {
					toasts.success('X is very flattered', 'X would like to do you too');
				} else {
					toasts.error('Cancelled');
				}
			}}>Confirm Strong</Button>
	</div>
	<div class="row">
		<Button info submit={() => prompt('Ice Cream', 'What is your favorite ice-cream flavour?').then((name) => toasts.info(`You like ${name}`))}>Single Prompt</Button>

		<Button
			info
			submit={() =>
				prompt('Validated Input', 'Write an odd number', undefined, undefined, (inputs) => parseInt(inputs[0].value) % 2 == 1).then((name) => toasts.info(`You like ${name}`))}>
			Single Validator Prompt
		</Button>

		<Button
			info
			submit={async () => {
				try {
					const answers = await prompt('What do you study?', ['Study Name', 'Year', 'Campus', ['Start Year', 2020]]);
					alert('Your answers', 'The following is the JSON object that was returned from your answers:', JSON.stringify(answers, 0, 2));
				} catch (err) {
					toasts.error('You ignored the input 😮');
				}
			}}>Multi Input</Button>
	</div>
</main>
