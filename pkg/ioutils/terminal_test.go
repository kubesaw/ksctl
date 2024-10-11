package ioutils_test

// func TestAskForConfirmationWhenAnswerIsY(t *testing.T) {
// 	for _, answer := range []string{"y", "Y"} {
// 		// given
// 		term := NewFakeTerminalWithResponse(answer)

// 		// when
// 		confirmation := term.AskForConfirmation(ioutils.WithMessagef("do some %s", "action"))

// 		// then
// 		assert.True(t, confirmation)
// 		output := term.Output()
// 		assert.Contains(t, output, "Are you sure that you want to do some action\n===============================\n[y/n] -> ")
// 		assert.NotContains(t, output, "!!!  DANGER ZONE  !!!")
// 	}
// }

// func TestAskForConfirmationWhenAssumeYesIsTrue(t *testing.T) {
// 	// given
// 	term := NewFakeTerminalWithResponse("n")
// 	ioutils.AssumeYes = true
// 	t.Cleanup(func() {
// 		ioutils.AssumeYes = false
// 	})

// 	// when
// 	confirmation := term.AskForConfirmation(ioutils.WithMessagef("do some %s", "action"))

// 	// then
// 	assert.True(t, confirmation)
// 	output := term.Output()
// 	assert.Contains(t, output, "Are you sure that you want to do some action\n===============================\n[y/n] -> ")
// 	assert.Contains(t, output, "[y/n] -> response: 'y'")
// 	assert.NotContains(t, output, "!!!  DANGER ZONE  !!!")
// }

// func TestAskForConfirmationWhenAnswerIsNWithDangerZone(t *testing.T) {
// 	for _, answer := range []string{"n", "N"} {
// 		// given
// 		term := NewFakeTerminalWithResponse(answer)

// 		// when
// 		confirmation := term.AskForConfirmation(ioutils.WithDangerZoneMessagef("a consequence", "do some %s", "action"))

// 		// then
// 		assert.False(t, confirmation)
// 		output := term.Output()
// 		assert.Contains(t, output, "!!!  DANGER ZONE  !!!")
// 		assert.Contains(t, output, "THIS COMMAND WILL CAUSE A CONSEQUENCE")
// 		assert.Contains(t, output, "Are you sure that you want to do some action\n===============================\n[y/n] -> ")
// 	}
// }

// func TestAskForConfirmationWhenFirstAnswerIsWrong(t *testing.T) {
// 	// given
// 	createTerm := func(correctAnswer string) ioutils.Terminal {
// 		counter := 0
// 		return ioutils.NewTerminal(
// 			func() io.Reader {
// 				in := bytes.NewBuffer(nil)
// 				if counter == 0 {
// 					in.WriteString("bla")
// 					counter++
// 				} else {
// 					in.WriteString(correctAnswer)
// 				}
// 				in.WriteByte('\n')
// 				return in
// 			},
// 			func() io.Writer {
// 				return bytes.NewBuffer(nil)
// 			},
// 		)
// 	}

// 	t.Run("second answer is y", func(t *testing.T) {
// 		// given
// 		term := createTerm("y")

// 		// when
// 		confirmation := term.AskForConfirmation(ioutils.WithMessagef("do some %s", "action"))

// 		// then
// 		assert.True(t, confirmation)
// 	})

// 	t.Run("second answer is n", func(t *testing.T) {
// 		// given
// 		term := createTerm("n")

// 		// when
// 		confirmation := term.AskForConfirmation(ioutils.WithMessagef("do some %s", "action"))

// 		// then
// 		assert.False(t, confirmation)
// 	})
// }
